package exec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boreec/boottime/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintRecordsAverage(t *testing.T) {
	tcs := map[string]struct {
		content  string
		prettify bool
		validate func(t *testing.T, err error)
	}{
		"valid records prettify false": {
			content: makeJSONL(t,
				model.BootTimeRecord{Values: map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
					model.BootTimeStageFirmware: {model.RetrievalMethodACPIFPDT: 1000000},
				}},
				model.BootTimeRecord{Values: map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
					model.BootTimeStageFirmware: {model.RetrievalMethodACPIFPDT: 3000000},
				}},
			),
			prettify: false,
			validate: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"valid records prettify true": {
			content: makeJSONL(t,
				model.BootTimeRecord{Values: map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
					model.BootTimeStageFirmware: {model.RetrievalMethodACPIFPDT: 1000000},
				}},
			),
			prettify: true,
			validate: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"empty file": {
			content: "",
			validate: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"nonexistent file": {
			validate: func(t *testing.T, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var path string
			if tc.content != "" || name == "empty file" {
				dir := t.TempDir()
				path = filepath.Join(dir, "test.jsonl")
				require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))
			} else {
				path = "/nonexistent/path/file.jsonl"
			}

			err := PrintRecordsAverage(path, tc.prettify)
			tc.validate(t, err)
		})
	}
}

func TestPrintRecordsAveragePrettier(t *testing.T) {
	tcs := map[string]struct {
		record   *model.BootTimeRecord
		validate func(t *testing.T, output string, err error)
	}{
		"table structure with header and data": {
			record: &model.BootTimeRecord{
				Values: map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
					model.BootTimeStageFirmware: {
						model.RetrievalMethodACPIFPDT: 1 * time.Second,
						model.RetrievalMethodEFIVar:   2 * time.Second,
					},
					model.BootTimeStageLoader: {
						model.RetrievalMethodACPIFPDT: 3 * time.Second,
					},
				},
			},
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.Contains(t, output, "Stage")
				assert.Contains(t, output, "acpi_fpdt")
				assert.Contains(t, output, "firmware")
				assert.Contains(t, output, "loader")
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.GreaterOrEqual(t, len(lines), 7)
			},
		},
		"empty record": {
			record: &model.BootTimeRecord{
				Values: map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{},
			},
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.Contains(t, output, "Stage")
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := printRecordsAveragePrettier(&buf, tc.record)
			tc.validate(t, buf.String(), err)
		})
	}
}

func makeJSONL(t *testing.T, records ...model.BootTimeRecord) string {
	t.Helper()
	var sb strings.Builder
	for _, r := range records {
		b, err := json.Marshal(r.Values)
		require.NoError(t, err)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}
