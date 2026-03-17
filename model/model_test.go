package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToTable(t *testing.T) {
	tcs := map[string]struct {
		record   BootTimeRecord
		validate func(t *testing.T, rows [][]string)
	}{
		"empty values": {
			record: BootTimeRecord{
				Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{},
			},
			validate: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 7) // header + 6 stages
				assert.Equal(t, "Stage", rows[0][0])
				for _, row := range rows[1:] {
					for _, cell := range row[1:] {
						assert.Equal(t, "", cell)
					}
				}
			},
		},
		"partial values one stage one method": {
			record: BootTimeRecord{
				Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {
						RetrievalMethodACPIFPDT: 5 * time.Second,
					},
				},
			},
			validate: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 7)
				assert.Equal(t, "firmware", rows[1][0])
				assert.Equal(t, (5 * time.Second).String(), rows[1][1])
				assert.Equal(t, "", rows[1][2])
				assert.Equal(t, "", rows[1][3])
				assert.Equal(t, "", rows[1][4])
			},
		},
		"fully populated": {
			record: BootTimeRecord{
				Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {
						RetrievalMethodACPIFPDT:       1 * time.Second,
						RetrievalMethodEFIVar:         2 * time.Second,
						RetrievalMethodSystemdDBUS:    3 * time.Second,
						RetrievalMethodSystemdAnalyze: 4 * time.Second,
					},
					BootTimeStageLoader: {
						RetrievalMethodACPIFPDT:       5 * time.Second,
						RetrievalMethodEFIVar:         6 * time.Second,
						RetrievalMethodSystemdDBUS:    7 * time.Second,
						RetrievalMethodSystemdAnalyze: 8 * time.Second,
					},
				},
			},
			validate: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 7)
				assert.Equal(t, (1 * time.Second).String(), rows[1][1])
				assert.Equal(t, (2 * time.Second).String(), rows[1][2])
				assert.Equal(t, (3 * time.Second).String(), rows[1][3])
				assert.Equal(t, (4 * time.Second).String(), rows[1][4])
				assert.Equal(t, (5 * time.Second).String(), rows[2][1])
			},
		},
		"header row correctness": {
			record: BootTimeRecord{
				Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{},
			},
			validate: func(t *testing.T, rows [][]string) {
				header := rows[0]
				assert.Equal(t, "Stage", header[0])
				assert.Equal(t, string(RetrievalMethodACPIFPDT), header[1])
				assert.Equal(t, string(RetrievalMethodEFIVar), header[2])
				assert.Equal(t, string(RetrievalMethodSystemdDBUS), header[3])
				assert.Equal(t, string(RetrievalMethodSystemdAnalyze), header[4])
			},
		},
		"stage ordering": {
			record: BootTimeRecord{
				Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{},
			},
			validate: func(t *testing.T, rows [][]string) {
				assert.Equal(t, "firmware", rows[1][0])
				assert.Equal(t, "loader", rows[2][0])
				assert.Equal(t, "kernel", rows[3][0])
				assert.Equal(t, "initrd", rows[4][0])
				assert.Equal(t, "userspace", rows[5][0])
				assert.Equal(t, "total", rows[6][0])
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows := tc.record.ToTable()
			tc.validate(t, rows)
		})
	}
}

func TestBootTimeAccumulator(t *testing.T) {
	tcs := map[string]struct {
		records  []*BootTimeRecord
		validate func(t *testing.T, avg *BootTimeRecord)
	}{
		"single record round-trip": {
			records: []*BootTimeRecord{
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 100 * time.Millisecond},
				}},
			},
			validate: func(t *testing.T, avg *BootTimeRecord) {
				assert.Equal(t, 100*time.Millisecond, avg.Values[BootTimeStageFirmware][RetrievalMethodACPIFPDT])
			},
		},
		"multiple records averaged": {
			records: []*BootTimeRecord{
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 100 * time.Millisecond},
				}},
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 200 * time.Millisecond},
				}},
			},
			validate: func(t *testing.T, avg *BootTimeRecord) {
				assert.Equal(t, 150*time.Millisecond, avg.Values[BootTimeStageFirmware][RetrievalMethodACPIFPDT])
			},
		},
		"integer division truncation": {
			records: []*BootTimeRecord{
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 1},
				}},
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 1},
				}},
				{Values: map[BootTimeStage]map[RetrievalMethod]time.Duration{
					BootTimeStageFirmware: {RetrievalMethodACPIFPDT: 2},
				}},
			},
			validate: func(t *testing.T, avg *BootTimeRecord) {
				// (1+1+2)/3 = 4/3 = 1 with integer division
				assert.Equal(t, time.Duration(1), avg.Values[BootTimeStageFirmware][RetrievalMethodACPIFPDT])
			},
		},
		"empty accumulator returns empty record": {
			records: []*BootTimeRecord{},
			validate: func(t *testing.T, avg *BootTimeRecord) {
				assert.Empty(t, avg.Values)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			acc := NewBootTimeAccumulator()
			for _, r := range tc.records {
				acc.Add(r)
			}
			avg := acc.Average()
			tc.validate(t, avg)
		})
	}
}

func TestBootTimeRecordsFromFile(t *testing.T) {
	tcs := map[string]struct {
		content  string
		validate func(t *testing.T, records []*BootTimeRecord, err error)
	}{
		"empty file": {
			content: "",
			validate: func(t *testing.T, records []*BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Empty(t, records)
			},
		},
		"single valid JSONL line": {
			content: `{"firmware":{"acpi_fpdt":1000000}}` + "\n",
			validate: func(t *testing.T, records []*BootTimeRecord, err error) {
				require.NoError(t, err)
				require.Len(t, records, 1)
				assert.Equal(t, time.Duration(1000000), records[0].Values[BootTimeStageFirmware][RetrievalMethodACPIFPDT])
			},
		},
		"multiple lines": {
			content: `{"firmware":{"acpi_fpdt":1000000}}` + "\n" + `{"firmware":{"acpi_fpdt":2000000}}` + "\n",
			validate: func(t *testing.T, records []*BootTimeRecord, err error) {
				require.NoError(t, err)
				require.Len(t, records, 2)
			},
		},
		"malformed JSON line returns error": {
			content: "not json\n",
			validate: func(t *testing.T, records []*BootTimeRecord, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "test.jsonl")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))

			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close()

			records, err := BootTimeRecordsFromFile(f)
			tc.validate(t, records, err)
		})
	}
}

func TestUnmarshalBootTimeRecord(t *testing.T) {
	tcs := map[string]struct {
		input    []byte
		validate func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"valid JSON round-trip": {
			input: []byte(`{"firmware":{"acpi_fpdt":5000000,"efi_var":6000000}}`),
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(5000000), rec.Values[BootTimeStageFirmware][RetrievalMethodACPIFPDT])
				assert.Equal(t, time.Duration(6000000), rec.Values[BootTimeStageFirmware][RetrievalMethodEFIVar])
			},
		},
		"malformed JSON": {
			input: []byte("{bad json}"),
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
			},
		},
		"empty input": {
			input: []byte(""),
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var rec BootTimeRecord
			err := UnmarshalBootTimeRecord(tc.input, &rec)
			tc.validate(t, &rec, err)
		})
	}
}
