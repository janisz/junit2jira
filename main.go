package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"github.com/hashicorp/go-multierror"
	"github.com/joshdk/go-junit"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
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
	flag.StringVar(&jiraUrl, "jira-url", "https://issues.redhat.com/", "Url of JIRA instance")
	flag.StringVar(&junitReportsDir, "junit-reports-dir", os.Getenv("ARTIFACT_DIR"), "Dir that contains jUnit reports XML files")
	flag.Parse()

	failedTests, err := findFailedTests(junitReportsDir, Env())
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

	err = createIssuesOrComments(failedTests, jiraClient)
	if err != nil {
		log.Fatal(err)
	}
}

func createIssuesOrComments(failedTests []testCase, jiraClient *jira.Client) error {
	var result error
	for _, tc := range failedTests {
		err := createIssueOrComment(jiraClient, tc)
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func createIssueOrComment(jiraClient *jira.Client, tc testCase) error {
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

func findFailedTests(dirName string, env map[string]string) ([]testCase, error) {
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
			failedTests = append(failedTests, NewTestCase(tc, env))
		}
	}
	log.Printf("Found %d failed tests", len(failedTests))
	return failedTests, nil
}

const (
	desc = `
{{- if .Message }}
{code:title=Message|borderStyle=solid}
{{ .Message }}
{code}
{{- end }}
{{- if .Stderr }}
{code:title=STDERR|borderStyle=solid}
{{ .Stderr }}
{code}
{{- end }}
{{- if .Stdout }}
{code:title=STDOUT|borderStyle=solid}
{{ .Stdout }}
{code}
{{- end }}

||    ENV     ||      Value           ||
| BUILD ID     | {{- .BuildId -}}      |
| BUILD TAG    | {{- .BuildTag -}}     |
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
	BuildId      string
	Cluster      string
	JobName      string
	Orchestrator string
	BuildTag     string
}

func NewTestCase(tc junit.Test, env map[string]string) testCase {
	return testCase{
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
	}
}

func (tc testCase) description() (string, error) {
	return render(tc, desc)
}

func (tc testCase) summary() (string, error) {
	return render(tc, summaryTpl)
}

func render(tc testCase, text string) (string, error) {
	tmpl, err := template.New("test").Parse(text)
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
