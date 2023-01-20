package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jimsnab/go-cmdline"
)

func main() {

	cl := cmdline.NewCommandLine()

	cl.RegisterCommand(
		scanHandler,
		"scan <string-startDate> [<string-endDate>]?Query from the specified start date (yyyy-mm-dd format) or date range",
		"--site <string-siteId>?The Imperva site ID - see Websites list on Imperva.",
		"*--code <string-eventCodes>?Provides a code or code prefix; must specify at least one.",
		"[--max-pages <int-maxPages>]?Limit the number of pages of events, default is 10",
	)

	cl.RegisterCommand(
		codesHandler,
		"codes <string-startDate> [<string-endDate>]?List rule codes encountered from the specified start date (yyyy-mm-dd format) or date range",
		"--site <string-siteId>?The Imperva site ID - see Websites list on Imperva.",
		"[--max-pages <int-maxPages>]?Limit the number of pages of events, default is 10",
	)

	cl.RegisterCommand(
		updateHandler,
		"update <path-dbPath>?Maintain aggregation state in a file; will not process partial days.",
		"--site <string-siteId>?The Imperva site ID - see Websites list on Imperva.",
		"*--code <string-eventCodes>?Provides a code or code prefix; must specify at least one.",
		"[--max-pages <int-maxPages>]?Limit the number of pages of events, default is 1000",
	)

	cl.RegisterCommand(
		viewHandler,
		"view <path-dbPath>?Dump the aggregation details in the specified database file in human-readable format.",
	)

	cl.RegisterCommand(
		csvHandler,
		"csv <path-dbPath>?Dump the aggregation details in the specified database file as a CSV",
		"[--no-heading]?Omit the first heading row",
	)

	args := os.Args[1:] // exclude executable name in os.Args[0]
	err := cl.Process(args)
	if err != nil {
		cl.Help(err, "tlsscan", args)
		fmt.Println("You must provide IMPERVA_API_KEY and IMPERVA_API_ID environment variables.")
		fmt.Println()
	}
}

func scanHandler(args cmdline.Values) (unused error) {
	siteId := args["siteId"].(string)
	rsDb := newRequestSourcesDb(siteId)

	fmt.Printf("Getting visits\n")

	maxPages := args["maxPages"].(int)
	if maxPages < 1 {
		maxPages = 10
	}

	startDate := args["startDate"].(string)
	endDate := args["endDate"].(string)

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		fmt.Printf("\nStart date '%s' is not valid\n\n", startDate)
		return
	}

	var end time.Time
	if endDate == "" {
		end = time.Now()
	} else {
		var err error
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			fmt.Printf("\nEnd date '%s' is not valid\n\n", endDate)
			return
		}
	}

	var codes []string
	c := args["eventCodes"]
	if c != nil {
		codes = c.([]string)
	}
	if len(codes) == 0 {
		fmt.Fprintf(os.Stderr, "\nSpecify at least one --code argument. Use the codes command to list possible codes.\n\n")
		return
	}

	visits, _, err := getVisits(siteId, start, end, maxPages, codes)
	if err != nil {
		return
	}

	for _, visit := range visits {
		err := rsDb.addVisit(visit)
		if err != nil {
			fmt.Println()
			fmt.Println(err)
			fmt.Println()
			return
		}
	}

	rsDb.print()
	return
}

func codesHandler(args cmdline.Values) (unused error) {
	siteId := args["siteId"].(string)
	rsDb := newRequestSourcesDb(siteId)

	fmt.Printf("Getting visits\n")

	maxPages := args["maxPages"].(int)
	if maxPages < 1 {
		maxPages = 10
	}

	startDate := args["startDate"].(string)
	endDate := args["endDate"].(string)

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		fmt.Printf("\nStart date '%s' is not valid\n\n", startDate)
		return
	}

	var end time.Time
	if endDate == "" {
		end = time.Now()
	} else {
		var err error
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			fmt.Printf("\nEnd date '%s' is not valid\n\n", endDate)
			return
		}
	}

	_, codes, err := getVisits(siteId, start, end, maxPages, nil)
	if err != nil {
		return
	}

	fmt.Println()
	if len(codes) > 0 {
		for code, count := range codes {
			if count == 1 {
				fmt.Printf("%s: %d hit\n", code, count)
			} else {
				fmt.Printf("%s: %d hits\n", code, count)
			}
		}
		fmt.Println()
	}

	if len(codes) == 1 {
		fmt.Printf("1 code found\n")
	} else {
		fmt.Printf("%d codes found\n", len(codes))
	}

	rsDb.print()
	return
}

func updateHandler(args cmdline.Values) (unused error) {
	dbPath := args["dbPath"].(string)
	siteId := args["siteId"].(string)

	now := time.Now()
	var start, end time.Time
	var rsDb *requestSourcesDb

	end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(-1 * time.Second)

	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		fmt.Printf("Creating %s\n", dbPath)
		start = end.Add(-30 * 24 * time.Hour)
		rsDb = newRequestSourcesDb(siteId)
		rsDb.dbPath = dbPath
	} else {
		fmt.Printf("Loading %s\n", dbPath)
		rsDb, err = loadRequestSourcesDb(siteId, dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return
		}

		if !end.After(rsDb.LastUpdate) {
			fmt.Println("Database is up to date.")
			return
		}

		start = rsDb.LastUpdate.Add(1 * time.Second)
	}

	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	if startDate == endDate {
		fmt.Printf("Updating - loading events from %s\n", startDate)
	} else {
		fmt.Printf("Updating - loading events from %s to %s\n", startDate, endDate)
	}

	var codes []string
	c := args["eventCodes"]
	if c != nil {
		codes = c.([]string)
	}
	if len(codes) == 0 {
		fmt.Fprintf(os.Stderr, "\nSpecify at least one --code argument. Use the codes command to list possible codes.\n\n")
		return
	}

	maxPages := args["maxPages"].(int)
	if maxPages < 1 {
		maxPages = 1000
	}

	visits, _, err := getVisits(siteId, start, end, maxPages, codes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	for _, visit := range visits {
		err := rsDb.addVisit(visit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return
		}
	}

	rsDb.LastUpdate = end

	fmt.Println("Saving changes")

	if err = rsDb.save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	fmt.Println("Update complete")
	return
}

func viewHandler(args cmdline.Values) (unused error) {
	dbPath := args["dbPath"].(string)

	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		fmt.Printf("\nFile does not exist: %s\n\n", dbPath)
		return
	}

	rsDb, err := loadRequestSourcesDb("", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	rsDb.print()

	fmt.Printf("Last update: %s\n\n", rsDb.LastUpdate.Format("2006-01-02"))
	return
}

func csvHandler(args cmdline.Values) (unused error) {
	dbPath := args["dbPath"].(string)

	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		fmt.Printf("\nFile does not exist: %s\n\n", dbPath)
		return
	}

	rsDb, err := loadRequestSourcesDb("", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	rsDb.dumpCsv(!(args["--no-heading"].(bool)))
	return
}
