package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/joshdk/go-junit"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	//go:embed testdata/slack/message-sample.xml
	messageSample []byte
	//go:embed testdata/slack/value-sample.xml
	valueSample []byte
	//go:embed testdata/slack/combined-sample.xml
	combinedSample []byte

	//go:embed testdata/slack/message-expected.json
	messageExpected []byte
	//go:embed testdata/slack/value-expected.json
	valueExpected []byte
	//go:embed testdata/slack/combined-expected.json
	combinedExpected []byte
)

func TestConstructSlackMessage(t *testing.T) {
	samples := [][]byte{messageSample, valueSample, combinedSample}
	expectations := [][]byte{messageExpected, valueExpected, combinedExpected}

	assert.Len(t, samples, len(expectations), "There are different amounts of samples and expected files. This a problem with the test rather than the code")

	j := junit2jira{}

	for i, sample := range samples {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			testsSuites, err := junit.Ingest(sample)
			assert.NoError(t, err)

			suites, err := j.findFailedTests(testsSuites)
			assert.NoError(t, err, "If this fails, it probably indicates a problem with the sample junit report rather than the code")
			assert.NotNil(t, suites, "If this fails, it probably indicates a problem with the sample junit report rather than the code")

			blocks := convertJunitToSlack(suites...)
			b, err := json.MarshalIndent(blocks, "", "  ")
			assert.NoError(t, err)
			assert.JSONEq(t, string(expectations[i]), string(b))
		})

	}
}
