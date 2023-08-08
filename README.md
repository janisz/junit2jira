# junit2jira

Convert test failures to jira issues

### Build
```shell
go build ./...
```

### Test
```shell
go test ./...
```

### Usage

```shell
Usage of junit2jira:
  -base-link string
    	Link to source code at the exact version under test.
  -build-id string
    	Build job run ID.
  -build-link string
    	Link to build job.
  -build-tag string
    	Built tag or revision.
  -csv-output string
    	Convert XML to a CSV file (use dash [-] for stdout)
  -dry-run
    	When set to true issues will NOT be created.
  -html-output string
    	Generate HTML report to this file (use dash [-] for stdout)
  -jira-url string
    	Url of JIRA instance (default "https://issues.redhat.com/")
  -job-name string
    	Name of CI job.
  -junit-reports-dir string
    	Dir that contains jUnit reports XML files
  -orchestrator string
    	Orchestrator name (such as GKE or OpenShift), if any.
  -threshold int
    	Number of reported failures that should cause single issue creation. (default 10)
  -timestamp string
    	Timestamp of CI test. (default "2023-04-18T12:07:44+02:00")
  -v	short alias for -version
  -version
    	print version information and exit
```

## Example usage
```shell
JIRA_TOKEN="..." junit2jira \
  -jira-url "https://..." \
  -junit-reports-dir "..." \
  -base-link "https://..." \
  -build-id "$BUILD_ID|GITHUB_RUN_ID" \
  -build-link "https://..." \
  -build-tag "$STACKROX_BUILD_TAG|$GITHUB_SHA" \
  -job-name "$JOB_NAME|$GITHUB_WORKFLOW" \
  -orchestrator "$ORCHESTRATOR_FLAVOR" \
  -timestamp $(date --rfc-3339=seconds)
  -csv-output -
```
