// Package junit defines a JUnit XML report and includes convenience methods
// for working with these reports.
package main

import (
	"encoding/xml"
)

// Testsuites is a collection of JUnit testsuites.
type Testsuites struct {
	XMLName xml.Name `xml:"testsuites"`

	Name     string `xml:"Name,attr,omitempty"`
	Time     string `xml:"time,attr,omitempty"` // total duration in seconds
	Tests    int    `xml:"tests,attr,omitempty"`
	Errors   int    `xml:"errors,attr,omitempty"`
	Failures int    `xml:"failures,attr,omitempty"`
	Skipped  int    `xml:"skipped,attr,omitempty"`
	Disabled int    `xml:"disabled,attr,omitempty"`

	Suites []Testsuite `xml:"testsuite,omitempty"`
}

// AddSuite adds a Testsuite and updates this testssuites' totals.
func (t *Testsuites) AddSuite(ts Testsuite) {
	t.Suites = append(t.Suites, ts)
	t.Tests += ts.Tests
	t.Errors += ts.Errors
	t.Failures += ts.Failures
	t.Skipped += ts.Skipped
	t.Disabled += ts.Disabled
}

// Testsuite is a single JUnit testsuite containing testcases.
type Testsuite struct {
	// required attributes
	Name     string `xml:"Name,attr"`
	Tests    int    `xml:"tests,attr"`
	Failures int    `xml:"failures,attr"`
	Errors   int    `xml:"errors,attr"`
	ID       int    `xml:"id,attr"`

	// optional attributes
	Disabled  int    `xml:"disabled,attr,omitempty"`
	Hostname  string `xml:"hostname,attr,omitempty"`
	Package   string `xml:"package,attr,omitempty"`
	Skipped   int    `xml:"skipped,attr,omitempty"`
	Time      string `xml:"time,attr"`                // duration in seconds
	Timestamp string `xml:"timestamp,attr,omitempty"` // date and time in ISO8601

	Properties *[]Property `xml:"properties>property,omitempty"`
	Testcases  []Testcase  `xml:"testcase,omitempty"`
	SystemOut  *Output     `xml:"system-out,omitempty"`
	SystemErr  *Output     `xml:"system-err,omitempty"`
}

// Testcase represents a single test with its results.
type Testcase struct {
	// required attributes
	Name      string `xml:"Name,attr"`
	Classname string `xml:"classname,attr"`

	// optional attributes
	Time   string `xml:"time,attr,omitempty"` // duration in seconds
	Status string `xml:"status,attr,omitempty"`

	Skipped   *Result `xml:"skipped,omitempty"`
	Error     *Result `xml:"error,omitempty"`
	Failure   *Result `xml:"failure,omitempty"`
	SystemOut *Output `xml:"system-out,omitempty"`
	SystemErr *Output `xml:"system-err,omitempty"`
}

// Property represents a key/value pair.
type Property struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"value,attr"`
}

// Result represents the result of a single test.
type Result struct {
	Message string `xml:"Message,attr"`
	Type    string `xml:"type,attr,omitempty"`
	Data    string `xml:",cdata"`
}

// Output represents output written to Stdout or sderr.
type Output struct {
	Data string `xml:",cdata"`
}
