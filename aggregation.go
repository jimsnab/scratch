package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type (
	entryPage struct {
		Url         string    `json:"url"`
		FirstAccess time.Time `json:"firstAccess"`
		LastAccess  time.Time `json:"lastAccess"`
	}

	requestSource struct {
		Ip    string            `json:"ip"`
		Info  map[string]string `json:"info"`
		Pages []*entryPage      `json:"pages"`
		Hits  int               `json:"hits"`
	}

	requestSourcesDb struct {
		Data       map[string]*requestSource `json:"data"`
		LastUpdate time.Time                 `json:"lastUpdate"`
		SiteId     string                    `json:"siteId"`
		dbPath     string
	}
)

func newRequestSourcesDb(siteId string) *requestSourcesDb {
	return &requestSourcesDb{
		Data:   map[string]*requestSource{},
		SiteId: siteId,
	}
}

func loadRequestSourcesDb(siteId, dbPath string) (*requestSourcesDb, error) {
	content, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, err
	}

	var rsDb requestSourcesDb
	err = json.Unmarshal(content, &rsDb)
	if err != nil {
		return nil, err
	}

	if siteId != "" && rsDb.SiteId != siteId {
		return nil, fmt.Errorf("the database at %s is for site %s, not %s", dbPath, rsDb.SiteId, siteId)
	}

	rsDb.dbPath = dbPath
	return &rsDb, nil
}

func (rsDb *requestSourcesDb) save() error {
	content, err := json.Marshal(rsDb)
	if err != nil {
		return err
	}

	return os.WriteFile(rsDb.dbPath, content, 0644)
}

func (rs *requestSourcesDb) addVisit(visit *impervaVisit) (err error) {
	for _, ip := range visit.ClientIPs {
		entry, exists := rs.Data[ip]
		if !exists {
			entry = &requestSource{
				Ip: ip,
			}
			rs.Data[ip] = entry

			var info map[string]string
			if info, err = whoIs(ip); err != nil {
				return
			}
			entry.Info = info
		}

		entry.Hits++

		var ep *entryPage
		for _, p := range entry.Pages {
			if p.Url == visit.EntryPage {
				ep = p
				break
			}
		}

		if ep == nil {
			ep = &entryPage{
				Url:         visit.EntryPage,
				FirstAccess: time.UnixMilli(visit.StartTime),
				LastAccess:  time.UnixMilli(visit.StartTime),
			}
			entry.Pages = append(entry.Pages, ep)
		} else {
			access := time.UnixMilli(visit.StartTime)
			if access.Before(ep.FirstAccess) {
				ep.FirstAccess = access
			}
			if access.After(ep.LastAccess) {
				ep.LastAccess = access
			}
		}
	}
	return
}

func (rsDb *requestSourcesDb) print() {
	if rsDb.SiteId != "" {
		fmt.Printf("Site: %s\n\n", rsDb.SiteId)
	}

	for ip, rs := range rsDb.Data {
		if len(rs.Pages) == 1 {
			for _, page := range rs.Pages {
				fromDate := page.FirstAccess.Format("2006-01-02")
				toDate := page.LastAccess.Format("2006-01-02")

				if fromDate != toDate {
					fmt.Printf("%s: %s - %d hits %s to %s\n", ip, page.Url, rs.Hits, fromDate, toDate)
				} else {
					fmt.Printf("%s: %s - %d hits %s\n", ip, page.Url, rs.Hits, fromDate)
				}
			}
		} else {
			fmt.Printf("%s:\n", ip)
			fmt.Printf("  Pages:\n")
			for _, page := range rs.Pages {
				fromDate := page.FirstAccess.Format("2006-01-02")
				toDate := page.LastAccess.Format("2006-01-02")

				if fromDate != toDate {
					fmt.Printf("  %s - %d hits %s to %s\n", page.Url, rs.Hits, fromDate, toDate)
				} else {
					fmt.Printf("  %s - %d hits %s\n", page.Url, rs.Hits, fromDate)
				}
			}
		}

		fields := []string{"OrgName", "OrgTechName", "OrgTechPhone", "OrgTechEmail", "Address", "City", "StateProv", "PostalCode", "Country"}

		for _, field := range fields {
			if _, exists := rs.Info[field]; exists {
				fmt.Printf("  %s: %s\n", field, rs.Info[field])
			}
		}
		fmt.Println()
	}
}

func escapeCsvCell(text string) string {
	if strings.ContainsAny(text, `",`) {
		return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
	} else {
		return text
	}
}

func (rsDb *requestSourcesDb) dumpCsv(hasHeading bool) {
	if hasHeading {
		fmt.Println("site,ip,url,hits,from,to,org-name,org-tech-name,org-tech-phone,org-tech-email,address,city,state,postal-code,country")
	}

	for ip, rs := range rsDb.Data {
		for _, page := range rs.Pages {
			fromDate := page.FirstAccess.Format("2006-01-02")
			toDate := page.LastAccess.Format("2006-01-02")

			fmt.Printf(
				"%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
				escapeCsvCell(rsDb.SiteId),
				escapeCsvCell(ip),
				escapeCsvCell(page.Url),
				rs.Hits,
				escapeCsvCell(fromDate),
				escapeCsvCell(toDate),
				escapeCsvCell(rs.Info["OrgName"]),
				escapeCsvCell(rs.Info["OrgTechName"]),
				escapeCsvCell(rs.Info["OrgTechPhone"]),
				escapeCsvCell(rs.Info["OrgTechEmail"]),
				escapeCsvCell(rs.Info["Address"]),
				escapeCsvCell(rs.Info["City"]),
				escapeCsvCell(rs.Info["StateProv"]),
				escapeCsvCell(rs.Info["PostalCode"]),
				escapeCsvCell(rs.Info["Country"]),
			)
		}
	}
}
