package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"github.com/hashicorp/go-multierror"
	"github.com/joshdk/go-junit"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"unicode"
)

const jql = `project in (ROX)
AND issuetype = Bug
AND status != Closed
AND labels = CI_Failure
AND summary ~ %q
ORDER BY created DESC`

func main() {

	jiraUrl := ""
	junitReportsDir := ""
	dryRun := false
	threshold := 0
	flag.StringVar(&jiraUrl, "jira-url", "https://issues.redhat.com/", "Url of JIRA instance")
	flag.StringVar(&junitReportsDir, "junit-reports-dir", os.Getenv("ARTIFACT_DIR"), "Dir that contains jUnit reports XML files")
	flag.BoolVar(&dryRun, "dry-run", false, "When set to true issues will NOT be created.")
	flag.IntVar(&threshold, "threshold", 10, "Number of reported failures that should cause single issue creation.")
	flag.Parse()

	failedTests, err := findFailedTests(junitReportsDir, Env(), threshold)
	if err != nil {
		log.Fatal(err)
	}

	transport := http.DefaultTransport

	tp := jira.PATAuthTransport{
		Token:     os.Getenv("JIRA_TOKEN"),
		Transport: transport,
	}

	jiraClient, err := jira.NewClient(tp.Client(), jiraUrl)
	if err != nil {
		log.Fatal(err)
	}

	err = createIssuesOrComments(failedTests, jiraClient, dryRun)
	if err != nil {
		log.Fatal(err)
	}
}

func createIssuesOrComments(failedTests []testCase, jiraClient *jira.Client, dryRun bool) error {
	var result error
	for _, tc := range failedTests {
		err := createIssueOrComment(jiraClient, tc, dryRun)
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func createIssueOrComment(jiraClient *jira.Client, tc testCase, dryRun bool) error {
	summary, err := tc.summary()
	if err != nil {
		return fmt.Errorf("could not get summary: %w", err)
	}
	description, err := tc.description()
	if err != nil {
		return fmt.Errorf("could not get description: %w", err)
	}
	log.Println("Searching for ", summary)
	search, response, err := jiraClient.Issue.Search(fmt.Sprintf(jql, summary), nil)
	if err != nil {
		logError(err, response)
		return fmt.Errorf("could not search: %w", err)
	}

	issue := findMatchingIssue(search, summary)

	if issue == nil {
		log.Println("Issue not found. Creating new issue...")
		log.Println(summary)
		log.Println(description)
		if dryRun {
			log.Println("Dry run: will just print issue content")
			log.Println(summary)
			log.Println(description)
			return nil
		}
		create, response, err := jiraClient.Issue.Create(newIssue(summary, description))
		if response != nil && err != nil {
			logError(err, response)
			return fmt.Errorf("could not create issue %s: %w", summary, err)
		}
		log.Printf("Created new issues: %s:%s", create.Key, summary)
		return nil
	}

	comment := jira.Comment{
		Body: description,
	}

	log.Printf("Found issue: %s %s. Creating a coment...", issue.ID, issue.Fields.Summary)

	if dryRun {
		log.Println("Dry run: will just print comment")
		log.Println(description)
		return nil
	}

	addComment, response, err := jiraClient.Issue.AddComment(issue.ID, &comment)
	if response != nil && err != nil {
		logError(err, response)
		return fmt.Errorf("could not create issue %s: %w", summary, err)
	}
	log.Printf("Created comment %s for %s:%s ", addComment.ID, issue.Key, summary)
	return nil
}

func newIssue(summary string, description string) *jira.Issue {
	return &jira.Issue{
		Fields: &jira.IssueFields{
			Type: jira.IssueType{
				Name: "Bug",
			},
			Project: jira.Project{
				Key: "ROX",
			},
			Summary:     summary,
			Description: description,
			Labels:      []string{"CI_Failure"},
		},
	}
}

func findMatchingIssue(search []jira.Issue, summary string) *jira.Issue {
	for _, i := range search {
		if i.Fields.Summary == summary {
			return &i
		}
	}
	return nil
}

func logError(err error, response *jira.Response) {
	log.Println(err)
	log.Println(response.StatusCode)
	all, err := io.ReadAll(response.Body)
	if err != nil {
		log.Println("Could not read body", err)
	} else {
		log.Println(string(all))
	}
}

func Env() map[string]string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i >= 0 {
			m[e[:i]] = e[i+1:]
		}
	}
	return m
}

func findFailedTests(dirName string, env map[string]string, threshold int) ([]testCase, error) {
	failedTests := make([]testCase, 0)
	testSuites, err := junit.IngestDir(dirName)
	if err != nil {
		return nil, fmt.Errorf("coud not read files: %w", err)
	}
	for _, ts := range testSuites {
		for _, tc := range ts.Tests {
			if tc.Error == nil {
				continue
			}
			failedTests = addFailedTest(failedTests, tc, env)
		}
	}
	log.Printf("Found %d failed tests", len(failedTests))

	if len(failedTests) > threshold && threshold > 0 {
		return mergeFailedTests(failedTests, env)
	}

	return failedTests, nil
}

func mergeFailedTests(failedTests []testCase, env map[string]string) ([]testCase, error) {
	log.Println("Too many failed tests, reporting them as a one failure.")
	msg := ""
	suite := failedTests[0].Suite
	for _, t := range failedTests {
		summary, err := t.summary()
		if err != nil {
			return nil, errors.Wrapf(err, "could not get summary of %+v", t)
		}
		// If there are multiple suites, do not report them.
		if suite != t.Suite {
			suite = ""
		}
		msg += summary + "\n"
	}
	tc := NewTestCase(junit.Test{
		Message:   msg,
		Classname: suite,
	}, env)
	return []testCase{tc}, nil
}

func addFailedTest(failedTests []testCase, tc junit.Test, env map[string]string) []testCase {
	if !isSubTest(tc) {
		return append(failedTests, NewTestCase(tc, env))
	}
	return addSubTestToFailedTest(tc, failedTests, env)
}

func isSubTest(tc junit.Test) bool {
	return strings.Contains(tc.Name, "/")
}

func addSubTestToFailedTest(subTest junit.Test, failedTests []testCase, env map[string]string) []testCase {
	// As long as the separator is not empty, split will always return a slice of length 1.
	name := strings.Split(subTest.Name, "/")[0]
	for i, failedTest := range failedTests {
		// Only consider a failed test a "parent" of the test if the name matches _and_ the class name is the same.
		if isGoTest(subTest.Classname) && failedTest.Name == name && failedTest.Suite == subTest.Classname {
			failedTest.addSubTest(subTest)
			failedTests[i] = failedTest
			return failedTests
		}
	}
	// In case we found no matches, we will default to add the subtest plain.
	return append(failedTests, NewTestCase(subTest, env))
}

// isGoTest will verify that the corresponding classname refers to a go package by expecting the go module name as prefix.
func isGoTest(className string) bool {
	return strings.HasPrefix(className, "github.com/stackrox/rox")
}

const (
	desc = `
{{- if .Message }}
{code:title=Message|borderStyle=solid}
{{ .Message | truncate }}
{code}
{{- end }}
{{- if .Stderr }}
{code:title=STDERR|borderStyle=solid}
{{ .Stderr | truncate }}
{code}
{{- end }}
{{- if .Stdout }}
{code:title=STDOUT|borderStyle=solid}
{{ .Stdout | truncate }}
{code}
{{- end }}
{{- if .Error }}
{code:title=ERROR|borderStyle=solid}
{{ .Error | truncate }}
{code}
{{- end }}

||    ENV     ||      Value           ||
| BUILD ID     | [{{- .BuildId -}}|https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/{{- .JobName -}}/{{- .BuildId -}}]|
| BUILD TAG    | [{{- .BuildTag -}}|{{- .BaseLink -}}]|
| JOB NAME     | {{- .JobName -}}      |
| CLUSTER      | {{- .Cluster -}}      |
| ORCHESTRATOR | {{- .Orchestrator -}} |
`
	summaryTpl = `{{ .Suite }} / {{ .Name }} FAILED`
)

type testCase struct {
	Name         string
	Suite        string
	Message      string
	Stdout       string
	Stderr       string
	Error        string
	BuildId      string
	Cluster      string
	JobName      string
	Orchestrator string
	BuildTag     string
	BaseLink     string
}

func NewTestCase(tc junit.Test, env map[string]string) testCase {
	jobSpec := env["JOB_SPEC"]
	baseLink := gjson.Get(jobSpec, "refs.base_link").String()
	c := testCase{
		Name:         tc.Name,
		Message:      tc.Message,
		Stdout:       tc.SystemOut,
		Stderr:       tc.SystemErr,
		Suite:        tc.Classname,
		BuildId:      env["BUILD_ID"],
		Cluster:      env["CLUSTER_NAME"],
		JobName:      env["JOB_NAME"],
		Orchestrator: env["ORCHESTRATOR_FLAVOR"],
		BuildTag:     env["STACKROX_BUILD_TAG"],
		BaseLink:     baseLink,
	}

	if tc.Error != nil {
		c.Error = tc.Error.Error()
	}
	return c
}

func (tc *testCase) description() (string, error) {
	return render(*tc, desc)
}

func (tc testCase) summary() (string, error) {
	s, err := render(tc, summaryTpl)
	if err != nil {
		return "", err
	}
	return clearString(s), nil
}

const subTestFormat = "\nSub test %s: %s"

func (tc *testCase) addSubTest(subTest junit.Test) {
	if subTest.Message != "" {
		tc.Message += fmt.Sprintf(subTestFormat, subTest.Name, subTest.Message)
	}
	if subTest.SystemOut != "" {
		tc.Stdout += fmt.Sprintf(subTestFormat, subTest.Name, subTest.SystemOut)
	}
	if subTest.SystemErr != "" {
		tc.Stderr += fmt.Sprintf(subTestFormat, subTest.Name, subTest.SystemErr)
	}
	if subTest.Error != nil {
		tc.Error += fmt.Sprintf(subTestFormat, subTest.Name, subTest.Error.Error())
	}
}

func render(tc testCase, text string) (string, error) {
	tmpl, err := template.New("test").Funcs(map[string]any{"truncate": truncate}).Parse(text)
	if err != nil {
		return "", err
	}
	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, tc)
	if err != nil {
		return "", err
	}
	return tpl.String(), nil
}

func clearString(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '/' || r == '-' || r == '_' {
			return r
		}
		return ' '
	}, str)
}

var maxTextBlockLength = 10000

func truncate(s string) string {
	runes := []rune(s)
	if len(runes) > maxTextBlockLength {
		return string(runes[:maxTextBlockLength]) + "\n … too long, truncated."
	}
	return s
}
