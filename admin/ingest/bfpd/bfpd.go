// Package bfpd implements an Ingest for Branded Food Products
package bfpd

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/littlebunch/gnutdata-bfpd-api/admin/ingest"
	"github.com/littlebunch/gnutdata-bfpd-api/admin/ingest/dictionaries"
	"github.com/littlebunch/gnutdata-bfpd-api/ds"
	fdc "github.com/littlebunch/gnutdata-bfpd-api/model"
)

var (
	cnts ingest.Counts
	err  error
)

// Bfpd for implementing the interface
type Bfpd struct {
	Doctype string
}

// ProcessFiles loads a set of Branded Food Products csv files processed
// in this order:
//		Products.csv  -- main food file
//		Servings.csv  -- servings sizes for each food
//		Nutrients.csv -- nutrient values for each food
func (p Bfpd) ProcessFiles(path string, dc ds.DataSource) error {
	var errs, errn error
	rcs, rcn := make(chan error), make(chan error)
	c1, c2 := true, true
	cnts.Foods, err = foods(path, dc, p.Doctype)
	if err != nil {
		log.Fatal(err)
	}

	go servings(path, dc, rcs)
	go nutrients(path, dc, rcn)
	for c1 || c2 {
		select {
		case errs, c1 = <-rcs:
			if c1 {
				if errs != nil {
					fmt.Printf("Error from servings: %v\n", errs)
				} else {
					fmt.Printf("Servings ingest complete.\n")
				}
			}

		case errn, c2 = <-rcn:
			if c2 {
				if err != nil {
					fmt.Printf("Error from nutrients: %v\n", errn)
				} else {
					fmt.Printf("Nutrient ingest complete.\n")
				}
			}
		}
	}
	log.Printf("Finished.  Counts: %d Foods %d Servings %d Nutrients\n", cnts.Foods, cnts.Servings, cnts.Nutrients)
	return err
}
func foods(path string, dc ds.DataSource, t string) (int, error) {
	var dt *fdc.DocType
	fn := path + "food.csv"
	cnt := 0
	f, err := os.Open(fn)
	if err != nil {
		return 0, err
	}
	r := csv.NewReader(f)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return cnt, err
		}
		cnts.Foods++
		if cnts.Foods%1000 == 0 {
			log.Println("Count = ", cnts.Foods)
		}
		pubdate, err := time.Parse("2006-01-02", record[4])
		if err != nil {
			log.Println(err)
		}
		dc.Update(record[0],
			fdc.Food{
				FdcID:           record[0],
				Description:     record[2],
				PublicationDate: pubdate,
				Type:            dt.ToString(fdc.FOOD),
			})
	}
	return cnts.Foods, err
}

func servings(path string, dc ds.DataSource, rc chan error) {
	defer close(rc)
	fn := path + "branded_food.csv"
	fgid := 0
	f, err := os.Open(fn)
	if err != nil {
		rc <- err
		return
	}
	r := csv.NewReader(f)
	cid := ""
	var (
		food fdc.Food
		s    []fdc.Serving
	)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rc <- err
			return
		}

		id := record[0]
		if cid != id {
			if cid != "" {
				food.Servings = s
				dc.Update(cid, food)
			}
			cid = id
			dc.Get(id, &food)
			food.Manufacturer = record[1]
			food.Upc = record[2]
			food.Ingredients = record[3]
			food.Source = record[8]
			if record[7] != "" {
				fgid++
				food.Group = &fdc.FoodGroup{ID: int32(fgid), Description: record[7], Type: "FGGPC"}
			} else {
				food.Group = nil
			}
			s = nil
		}

		cnts.Servings++
		if cnts.Servings%10000 == 0 {
			log.Println("Servings Count = ", cnts.Servings)
		}

		a, err := strconv.ParseFloat(record[4], 32)
		if err != nil {
			log.Println(record[0] + ": can't parse serving amount " + record[3])
		}
		s = append(s, fdc.Serving{
			Nutrientbasis: record[5],
			Description:   record[6],
			Servingamount: float32(a),
		})

	}
	rc <- err
	return
}
func nutrients(path string, dc ds.DataSource, rc chan error) {
	defer close(rc)
	var dt *fdc.DocType
	fn := path + "food_nutrient.csv"
	f, err := os.Open(fn)
	if err != nil {
		rc <- err
		return
	}
	r := csv.NewReader(f)
	cid := ""
	var (
		food fdc.Food
		n    []fdc.NutrientData
		il   interface{}
	)
	if err := dc.GetDictionary("gnutdata", dt.ToString(fdc.NUT), 0, 500, &il); err != nil {
		rc <- err
		return
	}

	nutmap := dictionaries.InitNutrientInfoMap(il)

	if err := dc.GetDictionary("gnutdata", dt.ToString(fdc.DERV), 0, 500, &il); err != nil {
		rc <- err
		return
	}
	dlmap := dictionaries.InitDerivationInfoMap(il)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rc <- err
			return
		}

		id := record[1]
		if cid != id {
			if cid != "" {
				food.Nutrients = n
				dc.Update(cid, food)
			}
			cid = id
			dc.Get(id, &food)
			n = nil
		}
		cnts.Nutrients++
		w, err := strconv.ParseFloat(record[3], 32)
		if err != nil {
			log.Println(record[0] + ": can't parse value " + record[4])
		}

		v, err := strconv.ParseInt(record[2], 0, 32)
		if err != nil {
			log.Println(record[0] + ": can't parse nutrient no " + record[1])
		}
		d, err := strconv.ParseInt(record[5], 0, 32)
		if err != nil {
			log.Println(record[5] + ": can't parse derivation no " + record[1])
		}
		var dv *fdc.Derivation
		if dlmap[uint(d)].Code != "" {
			dv = &fdc.Derivation{ID: dlmap[uint(d)].ID, Code: dlmap[uint(d)].Code, Type: dt.ToString(fdc.DERV), Description: dlmap[uint(d)].Description}
		} else {
			dv = nil
		}
		n = append(n, fdc.NutrientData{
			Nutrientno: nutmap[uint(v)].Nutrientno,
			Value:      float32(w),
			Nutrient:   nutmap[uint(v)].Name,
			Unit:       nutmap[uint(v)].Unit,
			Derivation: dv,
		})
		if cnts.Nutrients%30000 == 0 {
			log.Println("Nutrients Count = ", cnts.Nutrients)
		}

	}
	rc <- nil
	return
}
