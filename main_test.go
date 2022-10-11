package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseJunitReport(t *testing.T) {
	t.Run("not existing", func(t *testing.T) {
		tests, err := findFailedTests("not existing", nil)
		assert.Error(t, err)
		assert.Nil(t, tests)
	})
	t.Run("golang", func(t *testing.T) {
		tests, err := findFailedTests("testdata/report.xml", nil)
		assert.NoError(t, err)
		assert.Equal(t, []testCase{
			{
				Name:    "TestLocalScannerTLSIssuerIntegrationTests",
				Message: "Failed",
				Stdout:  "",
				Stderr:  "",
				Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
			},
			{
				Name:    "TestLocalScannerTLSIssuerIntegrationTests/TestSuccessfulRefresh",
				Message: "Failed",
				Stdout:  "",
				Stderr:  "",
				Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
			},
			{
				Name:    "TestLocalScannerTLSIssuerIntegrationTests/TestSuccessfulRefresh/no_secrets",
				Message: "Failed",
				Stdout:  "",
				Stderr:  "",
				Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
			},
		}, tests)
	})

	t.Run("dir", func(t *testing.T) {
		tests, err := findFailedTests("testdata", map[string]string{"BUILD_ID": "1"})
		assert.NoError(t, err)

		assert.ElementsMatch(
			t,
			[]testCase{
				{
					Name: "Verify policy Apache Struts: CVE-2017-5638 is triggered",
					Message: "Condition not satisfied:\n" +
						"\n" +
						"waitForViolation(deploymentName,  policyName, 60)\n" +
						"|                |                |\n" +
						"false            qadefpolstruts   Apache Struts: CVE-2017-5638\n" +
						"",
					Stdout: "?[1;30m21:35:15?[0;39m | ?[34mINFO ?[0;39m | DefaultPoliciesTest       | Starting testcase\n" +
						"?[1;30m21:36:16?[0;39m | ?[34mINFO ?[0;39m | Services                  | Failed to trigger Apache Struts: CVE-2017-5638 after waiting 60 seconds\n" +
						"?[1;30m21:36:16?[0;39m | ?[1;31mERROR?[0;39m | Helpers                   | An exception occurred in test\n" +
						"org.spockframework.runtime.ConditionNotSatisfiedError: Condition not satisfied:\n" +
						"\n" +
						"waitForViolation(deploymentName,  policyName, 60)\n" +
						"|                |                |\n" +
						"false            qadefpolstruts   Apache Struts: CVE-2017-5638\n" +
						"\n" +
						"\tat DefaultPoliciesTest.$spock_feature_1_0(DefaultPoliciesTest.groovy:181) [1 skipped]\n" +
						"\tat util.OnFailureInterceptor.intercept(OnFailure.groovy:72) [8 skipped]\n" +
						"\tat util.OnFailureInterceptor.intercept(OnFailure.groovy:72) [10 skipped]\n" +
						" [6 skipped]\n" +
						"?[1;30m21:36:16?[0;39m | ?[39mDEBUG?[0;39m | Helpers                   | 2022-09-30 21:36:16 Will collect various stackrox logs for this failure under /tmp/qa-tests-backend-logs/a57dc4b9-70eb-4391-8a00-c5948fef733d/\n" +
						"?[1;30m21:37:07?[0;39m | ?[39mDEBUG?[0;39m | Helpers                   | Ran: ./scripts/ci/collect-service-logs.sh stackrox /tmp/qa-tests-backend-logs/a57dc4b9-70eb-4391-8a00-c5948fef733d/stackrox-k8s-logs\n" +
						"Exit: 0\n",
					Stderr:  "",
					Suite:   "DefaultPoliciesTest",
					BuildId: "1",
				},
				{
					Name:    "TestLocalScannerTLSIssuerIntegrationTests",
					Message: "Failed",
					Stdout:  "",
					Stderr:  "",
					Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
					BuildId: "1",
				},
				{
					Name:    "TestLocalScannerTLSIssuerIntegrationTests/TestSuccessfulRefresh",
					Message: "Failed",
					Stdout:  "",
					Stderr:  "",
					Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
					BuildId: "1",
				},
				{
					Name:    "TestLocalScannerTLSIssuerIntegrationTests/TestSuccessfulRefresh/no_secrets",
					Message: "Failed",
					Stdout:  "",
					Stderr:  "",
					Suite:   "github.com/stackrox/rox/sensor/kubernetes/localscanner",
					BuildId: "1",
				},
			},
			tests,
		)
	})
	t.Run("gradle", func(t *testing.T) {
		tests, err := findFailedTests("testdata/TEST-DefaultPoliciesTest.xml", map[string]string{"BUILD_ID": "1"})
		assert.NoError(t, err)

		assert.Equal(
			t,
			[]testCase{{
				Name: "Verify policy Apache Struts: CVE-2017-5638 is triggered",
				Message: "Condition not satisfied:\n" +
					"\n" +
					"waitForViolation(deploymentName,  policyName, 60)\n" +
					"|                |                |\n" +
					"false            qadefpolstruts   Apache Struts: CVE-2017-5638\n" +
					"",
				Stdout: "?[1;30m21:35:15?[0;39m | ?[34mINFO ?[0;39m | DefaultPoliciesTest       | Starting testcase\n" +
					"?[1;30m21:36:16?[0;39m | ?[34mINFO ?[0;39m | Services                  | Failed to trigger Apache Struts: CVE-2017-5638 after waiting 60 seconds\n" +
					"?[1;30m21:36:16?[0;39m | ?[1;31mERROR?[0;39m | Helpers                   | An exception occurred in test\n" +
					"org.spockframework.runtime.ConditionNotSatisfiedError: Condition not satisfied:\n" +
					"\n" +
					"waitForViolation(deploymentName,  policyName, 60)\n" +
					"|                |                |\n" +
					"false            qadefpolstruts   Apache Struts: CVE-2017-5638\n" +
					"\n" +
					"\tat DefaultPoliciesTest.$spock_feature_1_0(DefaultPoliciesTest.groovy:181) [1 skipped]\n" +
					"\tat util.OnFailureInterceptor.intercept(OnFailure.groovy:72) [8 skipped]\n" +
					"\tat util.OnFailureInterceptor.intercept(OnFailure.groovy:72) [10 skipped]\n" +
					" [6 skipped]\n" +
					"?[1;30m21:36:16?[0;39m | ?[39mDEBUG?[0;39m | Helpers                   | 2022-09-30 21:36:16 Will collect various stackrox logs for this failure under /tmp/qa-tests-backend-logs/a57dc4b9-70eb-4391-8a00-c5948fef733d/\n" +
					"?[1;30m21:37:07?[0;39m | ?[39mDEBUG?[0;39m | Helpers                   | Ran: ./scripts/ci/collect-service-logs.sh stackrox /tmp/qa-tests-backend-logs/a57dc4b9-70eb-4391-8a00-c5948fef733d/stackrox-k8s-logs\n" +
					"Exit: 0\n",
				Stderr:  "",
				Suite:   "DefaultPoliciesTest",
				BuildId: "1",
			}},
			tests,
		)
	})
}

func TestDescription(t *testing.T) {
	tc := testCase{
		Name: "Verify policy Apache Struts: CVE-2017-5638 is triggered",
		Message: "Condition not satisfied:\n" +
			"\n" +
			"waitForViolation(deploymentName,  policyName, 60)\n" +
			"|                |                |\n" +
			"false            qadefpolstruts   Apache Struts: CVE-2017-5638\n" +
			"",
		Stdout: "?[1;30m21:35:15?[0;39m | ?[34mINFO ?[0;39m | DefaultPoliciesTest       | Starting testcase\n" +
			"?[1;30m21:36:16?[0;39m | ?[34mINFO ?[0;39m | Services                  | Failed to trigger Apache Struts: CVE-2017-5638 after waiting 60 seconds\n" +
			"?[1;30m21:36:16?[0;39m | ?[1;31mERROR?[0;39m | Helpers                   | An exception occurred in test\n" +
			"org.spockframework.runtime.ConditionNotSatisfiedError: Condition not satisfied:\n",
		Stderr:  "",
		Suite:   "DefaultPoliciesTest",
		BuildId: "1",
	}
	actual, err := tc.description()
	assert.NoError(t, err)
	assert.Equal(t, `
{code:title=Message|borderStyle=solid}
Condition not satisfied:

waitForViolation(deploymentName,  policyName, 60)
|                |                |
false            qadefpolstruts   Apache Struts: CVE-2017-5638

{code}
{code:title=STDOUT|borderStyle=solid}
?[1;30m21:35:15?[0;39m | ?[34mINFO ?[0;39m | DefaultPoliciesTest       | Starting testcase
?[1;30m21:36:16?[0;39m | ?[34mINFO ?[0;39m | Services                  | Failed to trigger Apache Struts: CVE-2017-5638 after waiting 60 seconds
?[1;30m21:36:16?[0;39m | ?[1;31mERROR?[0;39m | Helpers                   | An exception occurred in test
org.spockframework.runtime.ConditionNotSatisfiedError: Condition not satisfied:

{code}

||    ENV     ||      Value           ||
| BUILD ID     |1|
| BUILD TAG    ||
| JOB NAME     ||
| CLUSTER      ||
| ORCHESTRATOR ||
`, actual)
	s, err := tc.summary()
	assert.NoError(t, err)
	assert.Equal(t, `DefaultPoliciesTest / Verify policy Apache Struts: CVE-2017-5638 is triggered FAILED`, s)
}
