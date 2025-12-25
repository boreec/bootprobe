package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type RetrievalMethod string

const (
	RetrievalMethodEFIVar         RetrievalMethod = "efi_var"
	RetrievalMethodSystemdDBUS    RetrievalMethod = "systemd_dbus"
	RetrievalMethodSystemdAnalyze RetrievalMethod = "systemd_analyze"
)

var allRetrievalMethods = []RetrievalMethod{
	RetrievalMethodEFIVar,
	RetrievalMethodSystemdDBUS,
	RetrievalMethodSystemdAnalyze,
}

type BootTimeStage string

const (
	BootTimeStageFirmware  BootTimeStage = "firmware"
	BootTimeStageLoader    BootTimeStage = "loader"
	BootTimeStageKernel    BootTimeStage = "kernel"
	BootTimeStageInitrd    BootTimeStage = "initrd"
	BootTimeStageUserspace BootTimeStage = "userspace"
	BootTimeStageTotal     BootTimeStage = "total"
)

var allBootTimeStages = []BootTimeStage{
	BootTimeStageFirmware,
	BootTimeStageLoader,
	BootTimeStageKernel,
	BootTimeStageInitrd,
	BootTimeStageUserspace,
	BootTimeStageTotal,
}

type BootTimeRecord struct {
	Values map[BootTimeStage]map[RetrievalMethod]time.Duration
}

func (r BootTimeRecord) ToTable() [][]string {
	rows := make([][]string, 0, len(allBootTimeStages)+1)

	header := make([]string, 0, len(allRetrievalMethods)+1)
	header = append(header, "Stage")
	for _, m := range allRetrievalMethods {
		header = append(header, string(m))
	}
	rows = append(rows, header)

	for _, stage := range allBootTimeStages {
		row := make([]string, 0, len(allRetrievalMethods)+1)
		row = append(row, string(stage))

		methods, ok := r.Values[stage]
		for _, method := range allRetrievalMethods {
			if ok {
				if d, exists := methods[method]; exists {
					row = append(row, d.String())
					continue
				}
			}
			row = append(row, "")
		}
		rows = append(rows, row)
	}

	return rows
}

type BootTimeAccumulator struct {
	sum   map[BootTimeStage]map[RetrievalMethod]time.Duration
	count map[BootTimeStage]map[RetrievalMethod]int
}

func NewBootTimeAccumulator() *BootTimeAccumulator {
	return &BootTimeAccumulator{
		sum:   make(map[BootTimeStage]map[RetrievalMethod]time.Duration),
		count: make(map[BootTimeStage]map[RetrievalMethod]int),
	}
}

func (a *BootTimeAccumulator) Add(r *BootTimeRecord) {
	for stage, methods := range r.Values {
		if a.sum[stage] == nil {
			a.sum[stage] = make(map[RetrievalMethod]time.Duration)
			a.count[stage] = make(map[RetrievalMethod]int)
		}

		for method, d := range methods {
			a.sum[stage][method] += d
			a.count[stage][method]++
		}
	}
}

func (a *BootTimeAccumulator) Average() *BootTimeRecord {
	out := &BootTimeRecord{
		Values: make(map[BootTimeStage]map[RetrievalMethod]time.Duration),
	}

	for stage, methods := range a.sum {
		out.Values[stage] = make(map[RetrievalMethod]time.Duration)

		for method, total := range methods {
			out.Values[stage][method] = total / time.Duration(a.count[stage][method])
		}
	}

	return out
}

func BootTimeRecordsFromFile(file *os.File) ([]*BootTimeRecord, error) {
	records := []*BootTimeRecord{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()

		var rec BootTimeRecord
		if err := UnmarshalBootTimeRecord(line, &rec); err != nil {
			return nil, fmt.Errorf("unmarshalling boot time record from line: %w", err)
		}
		records = append(records, &rec)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func UnmarshalBootTimeRecord(line []byte, out *BootTimeRecord) error {
	var raw map[BootTimeStage]map[RetrievalMethod]time.Duration
	if err := json.Unmarshal(line, &raw); err != nil {
		return fmt.Errorf("unmarshalling from json: %w", err)
	}

	out.Values = make(map[BootTimeStage]map[RetrievalMethod]time.Duration)

	for bootTimeStage, methods := range raw {
		out.Values[bootTimeStage] = make(map[RetrievalMethod]time.Duration)

		for retrievalMethod, duration := range methods {
			out.Values[bootTimeStage][retrievalMethod] = duration
		}
	}

	return nil
}
