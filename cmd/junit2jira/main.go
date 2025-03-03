package main

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/stackrox/junit2jira/pkg/testcase"
	"html/template"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/andygrunwald/go-jira"
	"github.com/carlmjohnson/versioninfo"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joshdk/go-junit"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

const (
	jql = `project in (%s)
AND issuetype = Bug
AND status != Closed
AND labels = CI_Failure
AND summary ~ %q
ORDER BY created DESC`
	// Slack has a 150-character limit for text header
	slackHeaderTextLengthLimit = 150
	// Slack has a 3000-character limit for (non-field) text objects
	slackTextLengthLimit = 3000
)

func main() {
	var debug bool
	p := params{}
	var jiraUrl string
	flag.StringVar(&p.slackOutput, "slack-output", "", "Generate JSON output in slack format (use dash [-] for stdout)")
	flag.StringVar(&p.htmlOutput, "html-output", "", "Generate HTML report to this file (use dash [-] for stdout)")
	flag.StringVar(&p.csvOutput, "csv-output", "", "Convert XML to a CSV file (use dash [-] for stdout)")
	flag.StringVar(&p.summaryOutput, "summary-output", "", "Write a summary in JSON to this file (use dash [-] for stdout)")
	flag.StringVar(&jiraUrl, "jira-url", "https://issues.redhat.com/", "Url of JIRA instance")
	flag.StringVar(&p.jiraProject, "jira-project", "ROX", "The JIRA project for issues")
	flag.StringVar(&p.junitReportsDir, "junit-reports-dir", os.Getenv("ARTIFACT_DIR"), "Dir that contains jUnit reports XML files")
	flag.BoolVar(&p.dryRun, "dry-run", false, "When set to true issues will NOT be created.")
	flag.IntVar(&p.threshold, "threshold", 10, "Number of reported failures that should cause single issue creation.")
	flag.StringVar(&p.timestamp, "timestamp", time.Now().Format(time.RFC3339), "Timestamp of CI test.")
	flag.StringVar(&p.BaseLink, "base-link", "", "Link to source code at the exact version under test.")
	flag.StringVar(&p.BuildId, "build-id", "", "Build job run ID.")
	flag.StringVar(&p.BuildLink, "build-link", "", "Link to build job.")
	flag.StringVar(&p.BuildTag, "build-tag", "", "Built tag or revision.")
	flag.StringVar(&p.JobName, "job-name", "", "Name of CI job.")
	flag.StringVar(&p.Orchestrator, "orchestrator", "", "Orchestrator name (such as GKE or OpenShift), if any.")
	flag.BoolVar(&debug, "debug", false, "Enable debug log level")
	versioninfo.AddFlag(flag.CommandLine)
	flag.Parse()

	var err error

	p.jiraUrl, err = url.Parse(jiraUrl)
	if err != nil {
		log.Fatal(err)
	}

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	err = run(p)
	if err != nil {
		log.Fatal(err)
	}
}

type junit2jira struct {
	params
	jiraClient *jira.Client
}

type testIssue struct {
	issue    *jira.Issue
	newJIRA  bool
	testCase j2jTestCase
}

//region logger
type leveledLogger struct {
	*log.Logger
}

func (l leveledLogger) withFields(keysAndValues []interface{}) *log.Entry {
	f := make(map[string]interface{})

	for i := 0; i < len(keysAndValues)-1; i += 2 {
		f[keysAndValues[i].(string)] = keysAndValues[i+1]
	}

	return l.WithFields(f)
}

func (l leveledLogger) Error(msg string, keysAndValues ...interface{}) {
	l.withFields(keysAndValues).Error(msg)
}

func (l leveledLogger) Info(msg string, keysAndValues ...interface{}) {
	l.withFields(keysAndValues).Info(msg)
}
func (l leveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.withFields(keysAndValues).Debug(msg)
}

func (l leveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.withFields(keysAndValues).Warn(msg)
}

//endregion

func run(p params) error {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = retryablehttp.LeveledLogger(leveledLogger{log.StandardLogger()})
	transport := retryClient.StandardClient().Transport
	tp := jira.PATAuthTransport{
		Token:     os.Getenv("JIRA_TOKEN"),
		Transport: transport,
	}

	jiraClient, err := jira.NewClient(tp.Client(), p.jiraUrl.String())
	if err != nil {
		return errors.Wrapf(err, "could not create client for %s", p.jiraUrl)
	}

	j := &junit2jira{
		params:     p,
		jiraClient: jiraClient,
	}

	testSuites, err := testcase.LoadTestSuites(p.junitReportsDir)
	if err != nil {
		log.Fatalf("could not read files: %s", err)
	}

	err = j.createCsv(testSuites)
	if err != nil {
		log.Fatalf("could not create CSV: %s", err)
	}

	failedTests, err := j.getMergedFailedTests(testSuites)
	if err != nil {
		return errors.Wrap(err, "could not find failed tests")
	}

	issues, err := j.createIssuesOrComments(failedTests)
	if err != nil {
		return errors.Wrap(err, "could not create issues or comments")
	}

	err = j.createSlackMessage(issues)
	if err != nil {
		return errors.Wrap(err, "could not convert to slack")
	}

	jiraIssues := make([]*jira.Issue, 0, len(issues))
	for _, i := range issues {
		jiraIssues = append(jiraIssues, i.issue)
	}

	err = j.linkIssues(jiraIssues)
	if err != nil {
		return errors.Wrap(err, "could not link issues")
	}

	err = j.writeSummary(issues)
	if err != nil {
		return errors.Wrap(err, "could not write summary")
	}

	return errors.Wrap(j.createHtml(jiraIssues), "could not create HTML report")
}

func (j junit2jira) getMergedFailedTests(testSuites []junit.Suite) ([]j2jTestCase, error) {
	failedTests, err := testcase.GetFailedTests(testSuites)
	if err != nil {
		return nil, errors.Wrap(err, "could not get failed tests")
	}
	log.Infof("Found %d failed tests", len(failedTests))

	failedJ2jTests := make([]j2jTestCase, 0, len(failedTests))
	for _, failedTest := range failedTests {
		failedJ2jTests = append(failedJ2jTests, newJ2jTestCase(failedTest, j.params))
	}

	if 0 < j.threshold && j.threshold < len(failedTests) {
		failedJ2jTests, err = j.mergeFailedTests(failedJ2jTests)
		if err != nil {
			return nil, errors.Wrap(err, "could not merge failed tests")
		}
	}

	return failedJ2jTests, nil
}

//go:embed htmlOutput.html.tpl
var htmlOutputTemplate string

func (j junit2jira) createSlackMessage(tc []*testIssue) error {
	if j.slackOutput == "" {
		return nil
	}
	slackMsg := convertJunitToSlack(tc...)
	if slackMsg == nil {
		slackMsg = []slack.Attachment{}
	}

	b, err := json.Marshal(slackMsg)
	if err != nil {
		return fmt.Errorf("error while marshaling Slack message to json: %w", err)
	}
	out := os.Stdout
	if j.slackOutput != "-" {
		file, err := os.Create(j.slackOutput)
		if err != nil {
			return fmt.Errorf("could not create file %q: %w", j.slackOutput, err)
		}
		out = file
		defer file.Close()
	}
	_, err = fmt.Fprintf(out, "%s", string(b))
	if err != nil {
		return fmt.Errorf("error while marshaling Slack message to json: %w", err)
	}
	return nil
}

func (j junit2jira) createHtml(issues []*jira.Issue) error {
	if j.htmlOutput == "" || len(issues) == 0 {
		return nil
	}
	out := os.Stdout
	if j.htmlOutput != "-" {
		file, err := os.Create(j.htmlOutput)
		if err != nil {
			return fmt.Errorf("could not create file %q: %w", j.htmlOutput, err)
		}
		out = file
		defer file.Close()
	}
	return j.renderHtml(issues, out)
}

type htmlData struct {
	Issues  []*jira.Issue
	JiraUrl *url.URL
}

func (j junit2jira) renderHtml(issues []*jira.Issue, out io.Writer) error {
	t, err := template.New(j.htmlOutput).Parse(htmlOutputTemplate)
	if err != nil {
		return fmt.Errorf("could parse template: %w", err)
	}
	err = t.Execute(out, htmlData{
		Issues:  issues,
		JiraUrl: j.jiraUrl,
	})
	if err != nil {
		return fmt.Errorf("could not render template: %w", err)
	}
	return nil
}

func (j junit2jira) createCsv(testSuites []junit.Suite) error {
	if j.csvOutput == "" {
		return nil
	}
	out := os.Stdout
	if j.csvOutput != "-" {
		file, err := os.Create(j.csvOutput)
		if err != nil {
			return fmt.Errorf("could not create file %s: %w", j.csvOutput, err)
		}
		out = file
		defer file.Close()
	}
	return junit2csv(testSuites, j.params, out)
}

func (j junit2jira) createIssuesOrComments(failedTests []j2jTestCase) ([]*testIssue, error) {
	var result error
	issues := make([]*testIssue, 0, len(failedTests))
	for _, tc := range failedTests {
		issue, err := j.createIssueOrComment(tc)
		if err != nil {
			result = multierror.Append(result, err)
		}
		if issue != nil {
			issues = append(issues, issue)
		}
	}
	return issues, result
}

func (j junit2jira) linkIssues(issues []*jira.Issue) error {
	const linkType = "Related" // link type may vay between jira versions and configurations

	var result error
	for x, issue := range issues {
		for y := 0; y < x; y++ {
			// Skip cases where we have the same inward and outward issue.
			// Jira does not allow linking a ticket to itself.
			if issue.Key == issues[y].Key {
				continue
			}

			_, err := j.jiraClient.Issue.AddLink(&jira.IssueLink{
				Type:         jira.IssueLinkType{Name: linkType},
				OutwardIssue: &jira.Issue{Key: issue.Key},
				InwardIssue:  &jira.Issue{Key: issues[y].Key},
			})
			if err != nil {
				result = multierror.Append(result, err)
				continue
			}
			log.WithField("ID", issue.Key).Debugf("Created link to %s", issues[y].Key)
		}
	}
	return result
}

func (j junit2jira) createIssueOrComment(tc j2jTestCase) (*testIssue, error) {
	summary, err := tc.summary()
	if err != nil {
		return nil, fmt.Errorf("could not get summary: %w", err)
	}
	description, err := tc.description()
	if err != nil {
		return nil, fmt.Errorf("could not get description: %w", err)
	}
	const NA = "?"
	logEntry(NA, summary).Debug("Searching for issue")
	search, response, err := j.jiraClient.Issue.Search(fmt.Sprintf(jql, j.jiraProject, summary), nil)
	if err != nil {
		logError(err, response)
		return nil, fmt.Errorf("could not search: %w", err)
	}

	issue := findMatchingIssue(search, summary)
	issueWithTestCase := testIssue{
		issue:    issue,
		testCase: tc,
	}

	if issue == nil {
		logEntry(NA, summary).Info("Issue not found. Creating new issue...")
		if j.dryRun {
			logEntry(NA, summary).Debugf("Dry run: will just print issue\n %q", description)
			return nil, nil
		}
		issue = newIssue(j.jiraProject, summary, description)
		create, response, err := j.jiraClient.Issue.Create(issue)
		if response != nil && err != nil {
			logError(err, response)
			return nil, fmt.Errorf("could not create issue %s: %w", summary, err)
		}
		// Response from API does not contain full object so we need to copy missing data
		issue.Key = create.Key
		issue.ID = create.ID
		issue.Self = create.Self
		logEntry(issue.Key, summary).Info("Created new issue")
		issueWithTestCase.issue = issue
		issueWithTestCase.newJIRA = true
		return &issueWithTestCase, nil
	}

	comment := jira.Comment{
		Body: description,
	}

	logEntry(issue.Key, issue.Fields.Summary).Info("Found issue. Creating a comment...")

	if j.dryRun {
		logEntry(NA, issue.Fields.Summary).Debugf("Dry run: will just print comment:\n%q", description)
		return &issueWithTestCase, nil
	}

	addComment, response, err := j.jiraClient.Issue.AddComment(issue.ID, &comment)
	if response != nil && err != nil {
		logError(err, response)
		return nil, fmt.Errorf("could not create issue %s: %w", summary, err)
	}
	logEntry(issue.Key, summary).Infof("Created comment %s", addComment.ID)
	return &issueWithTestCase, nil
}

func (j junit2jira) writeSummary(tc []*testIssue) error {
	if j.summaryOutput == "" {
		return nil
	}
	out := os.Stdout
	if j.summaryOutput != "-" {
		file, err := os.Create(j.summaryOutput)
		if err != nil {
			return fmt.Errorf("could not create file %s: %w", j.summaryOutput, err)
		}
		out = file
		defer file.Close()
	}

	return generateSummary(tc, out)
}

type summary struct {
	NewJIRAs int `json:"newJIRAs"`
}

func generateSummary(tc []*testIssue, output io.Writer) error {
	newJIRAs := 0

	for _, testIssue := range tc {
		if testIssue.newJIRA {
			newJIRAs++
		}
	}
	summary := summary{
		NewJIRAs: newJIRAs,
	}

	json, err := json.Marshal(summary)
	if err != nil {
		return err
	}

	_, err = output.Write(json)

	return err
}

func logEntry(id, summary string) *log.Entry {

	return log.WithField("ID", id).WithField("summary", summary)
}

func newIssue(project string, summary string, description string) *jira.Issue {
	return &jira.Issue{
		Fields: &jira.IssueFields{
			Type: jira.IssueType{
				Name: "Bug",
			},
			Project: jira.Project{
				Key: project,
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

func logError(e error, response *jira.Response) {
	all, err := io.ReadAll(response.Body)

	if err != nil {
		log.WithError(e).WithField("StatusCode", response.StatusCode).Errorf("Could not read body: %q", err)
	} else {
		log.WithError(e).WithField("StatusCode", response.StatusCode).Error(string(all))
	}
}

func junit2csv(testSuites []junit.Suite, p params, output io.Writer) error {
	w := csv.NewWriter(output)
	header := []string{
		"BuildId",
		"Timestamp",
		"Classname",
		"Name",
		"Duration",
		"Status",
		"JobName",
		"BuildTag",
	}
	err := w.Write(header)
	if err != nil {
		return fmt.Errorf("coud not write header: %w", err)
	}
	for _, ts := range testSuites {
		if err = testSuiteToCSV(ts, p, w); err != nil {
			return err
		}
	}
	w.Flush()
	if w.Error() != nil {
		return fmt.Errorf("could not flush CSV: %w", w.Error())
	}
	return nil
}

func testSuiteToCSV(ts junit.Suite, p params, w *csv.Writer) error {
	for _, subTs := range ts.Suites {
		if err := testSuiteToCSV(subTs, p, w); err != nil {
			return err
		}
	}
	for _, tc := range ts.Tests {
		duration := fmt.Sprintf("%d", tc.Duration.Milliseconds())
		row := []string{
			p.BuildId,         // BuildId
			p.timestamp,       // Timestamp
			tc.Classname,      // Classname
			tc.Name,           // Name
			duration,          // Duration
			string(tc.Status), // Status
			p.JobName,         // JobName
			p.BuildTag,        // BuildTag
		}
		err := w.Write(row)
		if err != nil {
			return fmt.Errorf("coud not write row: %w", err)
		}
	}
	return nil
}

func (j junit2jira) mergeFailedTests(failedTests []j2jTestCase) ([]j2jTestCase, error) {
	log.Warning("Too many failed tests, reporting them as a one failure.")
	msg := ""
	suite := failedTests[0].Suite
	for _, t := range failedTests {
		summary, err := t.summary()
		if err != nil {
			return nil, errors.Wrapf(err, "could not get summary of %+v", t)
		}
		// If there are multiple suites, do not report them.
		if suite != t.Suite {
			suite = j.JobName
		}
		msg += summary + "\n"
	}

	tc := newJ2jTestCase(
		testcase.NewTestCase(
			junit.Test{
				Message:   msg,
				Classname: suite,
			}), j.params)

	return []j2jTestCase{tc}, nil
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
| BUILD ID     | [{{- .BuildId -}}|{{- .BuildLink -}}]|
| BUILD TAG    | [{{- .BuildTag -}}|{{- .BaseLink -}}]|
| JOB NAME     | {{- .JobName -}}      |
| ORCHESTRATOR | {{- .Orchestrator -}} |
`
	summaryTpl = `{{ (print .Suite " / " .Name) | truncateSummary }} FAILED`
)

type j2jTestCase struct {
	Name    string
	Suite   string
	Message string
	Stdout  string
	Stderr  string
	Error   string

	// Additional fields for junit2jira
	BuildId      string
	JobName      string
	Orchestrator string
	BuildTag     string
	BaseLink     string
	BuildLink    string
}

type params struct {
	BuildId      string
	JobName      string
	Orchestrator string
	BuildTag     string
	BaseLink     string
	BuildLink    string

	threshold       int
	dryRun          bool
	jiraUrl         *url.URL
	jiraProject     string
	junitReportsDir string
	timestamp       string
	csvOutput       string
	htmlOutput      string
	slackOutput     string
	summaryOutput   string
}

func newJ2jTestCase(testCase testcase.TestCase, p params) j2jTestCase {
	return j2jTestCase{
		Name:         testCase.Name,
		Suite:        testCase.Suite,
		Message:      testCase.Message,
		Stdout:       testCase.Stdout,
		Stderr:       testCase.Stderr,
		Error:        testCase.Error,
		BuildId:      p.BuildId,
		JobName:      p.JobName,
		Orchestrator: p.Orchestrator,
		BuildTag:     p.BuildTag,
		BaseLink:     p.BaseLink,
		BuildLink:    p.BuildLink,
	}
}

func (tc *j2jTestCase) description() (string, error) {
	return render(*tc, desc)
}

func (tc j2jTestCase) summary() (string, error) {
	s, err := render(tc, summaryTpl)
	if err != nil {
		return "", err
	}
	return clearString(s), nil
}

func render(tc j2jTestCase, text string) (string, error) {
	tmpl, err := template.New("test").Funcs(map[string]any{"truncate": truncate, "truncateSummary": truncateSummary}).Parse(text)
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

var maxSummaryLength = 200

func truncateSummary(s string) string {
	runes := []rune(s)
	if len(runes) > maxSummaryLength {
		return string(runes[:maxSummaryLength]) + "..."
	}
	return s
}

func convertJunitToSlack(issues ...*testIssue) []slack.Attachment {
	var failedTestsBlocks []slack.Block
	var attachments []slack.Attachment

	for _, i := range issues {
		var title string
		tc := i.testCase
		if tc.Suite == "" {
			title = tc.Name
		} else {
			title = fmt.Sprintf("%s: %s", tc.Suite, tc.Name)
		}

		issue := i.issue
		if issue != nil {
			title = fmt.Sprintf("%s: %s", issue.Key, title)
		}
		title = crop(title, slackHeaderTextLengthLimit)

		titleTextBlock := slack.NewTextBlockObject("plain_text", title, false, false)
		titleSectionBlock := slack.NewSectionBlock(titleTextBlock, nil, nil)
		failedTestsBlocks = append(failedTestsBlocks, titleSectionBlock)

		failureAttachment, err := failureToAttachment(title, tc)
		if err != nil {
			log.Printf("skipping %s: %v", tc.Name, err)
			continue
		}

		attachments = append(attachments, failureAttachment)

		// We've reached the desired message limit. We need to break out of all the loops
		if len(attachments) > 3 {
			break
		}
	}

	if len(failedTestsBlocks) == 0 {
		return nil
	}

	headerTextBlock := slack.NewTextBlockObject("plain_text", "Failed tests", false, false)
	headerBlock := slack.NewHeaderBlock(headerTextBlock)
	// Push this block to the beginning of the slice
	failedTestsBlocks = append([]slack.Block{headerBlock}, failedTestsBlocks...)

	failedTestsAttachment := slack.Attachment{
		Color:  "#bb2124",
		Blocks: slack.Blocks{BlockSet: failedTestsBlocks},
	}
	// Push this block to the beginning of the slice
	attachments = append([]slack.Attachment{failedTestsAttachment}, attachments...)

	return attachments
}

func failureToAttachment(title string, tc j2jTestCase) (slack.Attachment, error) {

	failureMessage := tc.Message
	failureValue := tc.Error
	if tc.Error == tc.Message {
		failureValue = ""
	}

	if failureMessage == "" && failureValue == "" {
		return slack.Attachment{}, fmt.Errorf("no junit failure message or error for %s", title)
	}

	failureMessage = crop(failureMessage, slackTextLengthLimit)
	failureValue = crop(failureValue, slackTextLengthLimit)

	// Add some formatting to the failure title
	failureTitleTextBlock := slack.NewTextBlockObject("plain_text", title, false, false)
	failureTitleHeaderBlock := slack.NewHeaderBlock(failureTitleTextBlock)

	failureAttachment := slack.Attachment{
		Color:  "#bb2124",
		Blocks: failureToBlocks(failureTitleHeaderBlock, failureMessage, failureValue),
	}
	return failureAttachment, nil
}

func failureToBlocks(failureTitleHeaderBlock *slack.HeaderBlock, messageText, valueText string) slack.Blocks {
	if messageText == "" && valueText == "" {
		return slack.Blocks{}
	}

	if messageText == "" {
		infoTextBlock := slack.NewTextBlockObject("mrkdwn", "*Info*", false, false)
		infoSectionBlock := slack.NewSectionBlock(infoTextBlock, nil, nil)

		failureValueTextBlock := slack.NewTextBlockObject("plain_text", valueText, false, false)
		failureValueSectionBlock := slack.NewSectionBlock(failureValueTextBlock, nil, nil)

		return slack.Blocks{BlockSet: []slack.Block{
			failureTitleHeaderBlock,
			infoSectionBlock,
			failureValueSectionBlock,
		}}
	}

	messageTextBlock := slack.NewTextBlockObject("mrkdwn", "*Message*", false, false)
	messageSectionBlock := slack.NewSectionBlock(messageTextBlock, nil, nil)

	failureMessageTextBlock := slack.NewTextBlockObject("plain_text", messageText, false, false)
	failureMessageSectionBlock := slack.NewSectionBlock(failureMessageTextBlock, nil, nil)

	if valueText == "" {
		return slack.Blocks{BlockSet: []slack.Block{
			failureTitleHeaderBlock,
			messageSectionBlock,
			failureMessageSectionBlock,
		}}
	}

	additionalInfoTextBlock := slack.NewTextBlockObject("mrkdwn", "*Additional Info*", false, false)
	additionalInfoSectionBlock := slack.NewSectionBlock(additionalInfoTextBlock, nil, nil)

	failureValueTextBlock := slack.NewTextBlockObject("plain_text", valueText, false, false)
	failureValueSectionBlock := slack.NewSectionBlock(failureValueTextBlock, nil, nil)

	return slack.Blocks{BlockSet: []slack.Block{
		failureTitleHeaderBlock,
		messageSectionBlock,
		failureMessageSectionBlock,
		additionalInfoSectionBlock,
		failureValueSectionBlock,
	}}
}

func crop(s string, l int) string {
	if len(s) < l {
		return s
	}
	return s[:l-1] + "…"
}
