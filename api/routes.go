package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/littlebunch/gnutdata-bfpd-api/model"
)

func countsGet(c *gin.Context) {
	var counts []interface{}
	t := c.Param("doctype")
	if t == "" {
		if t = c.Query("doctype"); t == "" {
			errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "Count parameter is required!"})
			return
		}
	}
	if err := dc.Counts(cs.CouchDb.Bucket, t, &counts); err != nil {
		errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No counts found!"})
		return
	}
	c.JSON(http.StatusOK, counts[0])
	return
}

// foodFdcID returns a single food based on a key value constructed from the fdcid
// If the format parameter equals 'meta' then only the food's meta-data is returned.
func foodFdcID(c *gin.Context) {
	q := c.Param("id")
	if q == "" {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "a FDC id in the q parameter is required"})
		return
	}
	if c.Query("format") == fdc.META {
		var f fdc.FoodMeta
		err := dc.Get(q, &f)
		if err != nil {
			errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No food found!"})
		} else {
			c.JSON(http.StatusOK, f)
		}
	} else {
		var f fdc.Food
		err := dc.Get(q, &f)
		if err != nil {
			errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No food found!"})
		}
		if c.Query("format") == fdc.SERVING {
			c.JSON(http.StatusOK, f.Servings)
		} else if c.Query("format") == fdc.NUTRIENTS {
			c.JSON(http.StatusOK, f.Nutrients)
		} else {
			c.JSON(http.StatusOK, f)
		}
	}

	return
}

// foodsBrowse returns a BrowseResult
func foodsBrowse(c *gin.Context) {
	var (
		max  int64
		page int64
		//count  int
		format string
		sort   string
		foods  []interface{}
		dt     fdc.DocType
	)
	// check the format parameter which defaults to META if not set
	if format = c.Query("format"); format == "" {
		format = fdc.META
	}
	if format != fdc.FULL && format != fdc.META && format != fdc.SERVING && format != fdc.NUTRIENTS {
		errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": fmt.Sprintf("valid formats are %s, %s, %s or %s", fdc.META, fdc.FULL, fdc.SERVING, fdc.NUTRIENTS)})
		return
	}
	if sort = c.Query("sort"); sort == "" {
		sort = "fdcId"
	}
	if sort != "" && sort != "foodDescription" && sort != "company" && sort != "fdcId" {
		errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "Unrecognized sort parameter.  Must be 'company', 'name' or 'fdcId'"})
		return
	}
	source := c.Query("source")
	if source != "" && dt.ToDocType(source) == 999 {
		errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": fmt.Sprintf("Unrecognized source parameter.  Must be %s, %s or %s", dt.ToString(fdc.BFPD), dt.ToString(fdc.SR), dt.ToString(fdc.FNDDS))})
		return
	}
	if max, err = strconv.ParseInt(c.Query("max"), 10, 32); err != nil {
		max = defaultListMax
	}
	if max > maxListSize {
		errorout(c, http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": fmt.Sprintf("max parameter %d exceeds maximum allowed size of %d", max, maxListSize)})
		return
	}
	if page, err = strconv.ParseInt(c.Query("page"), 10, 32); err != nil {
		page = 0
	}
	if page < 0 {
		page = 0
	}
	offset := page * max
	where := sourceFilter(source)
	dc.Browse(cs.CouchDb.Bucket, where, offset, max, format, sort, &foods)
	results := fdc.BrowseResult{Count: int32(len(foods)), Start: int32(page), Max: int32(max), Items: foods}
	c.JSON(http.StatusOK, results)
}

// foodsSearch runs a search and returns a BrowseResult
func foodsSearch(c *gin.Context) {
	var (
		max    int
		page   int
		format string
		foods  []interface{}
	)
	count := 0
	// check for a query
	q := c.Query("q")
	if q == "" {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "A search string in the q parameter is required"})
		return
	}
	// check for field
	f := c.Query("f")
	if f != "" && f != "foodDescription" && f != "upc" && f != "company" && f != "ingredients" {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "Unrecognized search field.  Must be one of 'foodDescription','company', 'upc' or 'ingredients'"})
		return
	}
	// check the format parameter which defaults to BRIEF if not set
	if format = c.Query("format"); format == "" {
		format = fdc.META
	}
	if format != fdc.FULL && format != fdc.META && format != fdc.SERVING && format != fdc.NUTRIENTS {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("valid formats are %s, %s, %s or %s", fdc.META, fdc.FULL, fdc.SERVING, fdc.NUTRIENTS)})
		return
	}
	if max, err = strconv.Atoi(c.Query("max")); err != nil {
		max = defaultListMax
	}
	if max > maxListSize {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("max parameter %d exceeds maximum allowed size of %d", max, maxListSize)})
		return
	}
	if page, err = strconv.Atoi(c.Query("page")); err != nil {
		page = 0
	}
	if page < 0 {
		page = 0
	}
	offset := page * max

	if count, err = dc.Search(fdc.SearchRequest{Query: q, IndexName: cs.CouchDb.Fts, Format: format, Max: max, Page: offset}, &foods); err != nil {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("Search query failed %v", err)})
		return
	}
	results := fdc.BrowseResult{Count: int32(count), Start: int32(page), Max: int32(max), Items: foods}
	c.JSON(http.StatusOK, results)
}

// foodsSearch runs a search as a POST and returns a BrowseResult
func foodsSearchPost(c *gin.Context) {
	var (
		foods []interface{}
		sr    fdc.SearchRequest
	)
	count := 0
	// check for a query
	err = c.BindJSON(&sr)
	if err != nil {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": err})
		return
	}
	if sr.Query == "" {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "Search query is required."})
		return
	}

	// check the format parameter which defaults to BRIEF if not set
	if sr.Format == "" {
		sr.Format = fdc.META
	} else if sr.Format != fdc.FULL && sr.Format != fdc.SERVING && sr.Format != fdc.NUTRIENTS {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("valid formats are %s, %s, %s or %s", fdc.META, fdc.FULL, fdc.SERVING, fdc.NUTRIENTS)})
		return
	}
	if &sr.Max == nil {
		sr.Max = defaultListMax
	} else if sr.Max > maxListSize || sr.Max < 0 {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("max parameter %d must be > 0 or <=  %d", sr.Max, maxListSize)})
		return
	}
	if &sr.Page == nil {
		sr.Page = 0
	}
	if sr.Page < 0 {
		sr.Page = 0
	}
	sr.Page = sr.Page * sr.Max
	sr.IndexName = cs.CouchDb.Fts
	if count, err = dc.Search(sr, &foods); err != nil {
		errorout(c, http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": fmt.Sprintf("Search query failed %v", err)})
		return
	}
	results := fdc.BrowseResult{Count: int32(count), Start: int32(sr.Page), Max: int32(sr.Max), Items: foods}
	c.JSON(http.StatusOK, results)
}

// errorout
func errorout(c *gin.Context, status int, data gin.H) {
	switch c.Request.Header.Get("Accept") {
	case "application/xml":
		c.XML(status, data)
	default:
		c.JSON(status, data)
	}
}
func sourceFilter(s string) string {
	w := ""
	if s != "" {
		if s == "BFPD" {
			w = fmt.Sprintf(" AND ( dataSource = '%s' OR dataSource='%s' )", "LI", "GTSN")
		} else {
			w = fmt.Sprintf(" AND dataSource = '%s'", s)
		}
	}
	return w
}
