// Package cdr parses the Constancia de Recepción (CDR) SUNAT returns: a UBL
// ApplicationResponse, usually inside a ZIP archive.
package cdr

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// maxEntry caps the decompressed CDR size to guard against zip bombs.
const maxEntry = 10 << 20

// Response is the outcome SUNAT reports for one document.
type Response struct {
	ID          string   // CDR identifier
	ReferenceID string   // Document the CDR answers, e.g. "F001-1"
	Code        string   // ResponseCode: "0" accepted, 4000+ accepted with observations
	Description string   // Human-readable outcome
	Notes       []string // Observations, e.g. "4252 - ..."
	XML         []byte   // Raw CDR XML, to be stored as proof
}

// Accepted reports whether SUNAT accepted the document. Codes 4000 and above
// are observations: the document is valid but should be corrected next time.
func (r Response) Accepted() bool {
	n, err := strconv.Atoi(r.Code)
	if err != nil {
		return false
	}
	return n == 0 || n >= 4000
}

type applicationResponse struct {
	ID       string   `xml:"ID"`
	Notes    []string `xml:"Note"`
	Response *struct {
		ReferenceID string `xml:"ReferenceID"`
		Code        string `xml:"ResponseCode"`
		Description string `xml:"Description"`
	} `xml:"DocumentResponse>Response"`
}

// Parse reads a CDR XML document.
func Parse(data []byte) (Response, error) {
	var ar applicationResponse
	if err := xml.Unmarshal(data, &ar); err != nil {
		return Response{}, fmt.Errorf("cdr: parse xml: %w", err)
	}
	if ar.Response == nil || strings.TrimSpace(ar.Response.Code) == "" {
		return Response{}, errors.New("cdr: missing DocumentResponse/Response/ResponseCode")
	}
	r := Response{
		ID:          strings.TrimSpace(ar.ID),
		ReferenceID: strings.TrimSpace(ar.Response.ReferenceID),
		Code:        strings.TrimSpace(ar.Response.Code),
		Description: strings.TrimSpace(ar.Response.Description),
		XML:         data,
	}
	for _, n := range ar.Notes {
		if n = strings.TrimSpace(n); n != "" {
			r.Notes = append(r.Notes, n)
		}
	}
	return r, nil
}

// ParseZip extracts the CDR XML from a SUNAT ZIP (skipping directory entries
// such as "dummy/") and parses it. Entries named "R-*.xml" are preferred.
func ParseZip(data []byte) (Response, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Response{}, fmt.Errorf("cdr: open zip: %w", err)
	}
	var chosen *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(path.Ext(f.Name), ".xml") {
			continue
		}
		if strings.HasPrefix(path.Base(f.Name), "R-") {
			chosen = f
			break
		}
		if chosen == nil {
			chosen = f
		}
	}
	if chosen == nil {
		return Response{}, errors.New("cdr: zip has no xml file")
	}
	rc, err := chosen.Open()
	if err != nil {
		return Response{}, fmt.Errorf("cdr: open %s: %w", chosen.Name, err)
	}
	defer rc.Close()
	xmlData, err := io.ReadAll(io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return Response{}, fmt.Errorf("cdr: read %s: %w", chosen.Name, err)
	}
	if len(xmlData) > maxEntry {
		return Response{}, fmt.Errorf("cdr: %s exceeds %d bytes", chosen.Name, maxEntry)
	}
	return Parse(xmlData)
}
