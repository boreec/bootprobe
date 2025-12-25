package exec

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/boreec/boottime/model"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sync/errgroup"
)

const efivarsPath string = "/sys/firmware/efi/efivars"

type BootTimeRecordWithEFIVariables struct {
	Firmware time.Duration
	Loader   time.Duration
}

func RetrieveBootTimeWithEFIVars() (*BootTimeRecordWithEFIVariables, error) {
	entries, err := os.ReadDir(efivarsPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", efivarsPath, err)
	}

	var initPath, execPath string
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "LoaderTimeInitUSec-"):
			initPath = filepath.Join(efivarsPath, name)
		case strings.HasPrefix(name, "LoaderTimeExecUSec-"):
			execPath = filepath.Join(efivarsPath, name)
		}

		if initPath != "" && execPath != "" {
			break
		}
	}

	if initPath == "" || execPath == "" {
		return nil, fmt.Errorf("EFI loader timing variables not found")
	}

	initRaw, err := readEFIVarValue(initPath)
	if err != nil {
		return nil, err
	}
	execRaw, err := readEFIVarValue(execPath)
	if err != nil {
		return nil, err
	}

	initTime, err := parseEFIMicroseconds(initRaw)
	if err != nil {
		return nil, err
	}
	execTime, err := parseEFIMicroseconds(execRaw)
	if err != nil {
		return nil, err
	}

	if execTime < initTime {
		return nil, fmt.Errorf("EFI loader exec time < init time")
	}

	return &BootTimeRecordWithEFIVariables{
		Firmware: initTime,
		Loader:   execTime - initTime,
	}, nil
}

func readEFIVarValue(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}
	if len(data) < 4 {
		return nil, errors.New("EFI var too short")
	}
	// first 4 bytes are attributes; skip them
	return data[4:], nil
}

func parseEFIMicroseconds(data []byte) (time.Duration, error) {
	if len(data)%2 != 0 {
		return 0, fmt.Errorf("invalid UTF-16 length")
	}

	// decode UTF-16 LE digits
	runes := make([]rune, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		v := binary.LittleEndian.Uint16(data[i:])
		if v == 0 {
			break // NUL-terminated
		}
		runes = append(runes, rune(v))
	}

	us, err := strconv.ParseInt(string(runes), 10, 64)
	if err != nil {
		return 0, err
	}

	return time.Duration(us) * time.Microsecond, nil
}

type BootTimeRecordWithSystemd struct {
	Firmware  time.Duration
	Loader    time.Duration
	Kernel    time.Duration
	Initrd    time.Duration
	Userspace time.Duration
	Total     time.Duration
}

func RetrieveBootTimeWithSystemdAnalyze() (*BootTimeRecordWithSystemd, error) {
	cmd := exec.Command("systemd-analyze", "time")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	return ParseSystemdAnalyzeTimeOutput(string(out))
}

func RetrieveBootTimeWithDbus() (*BootTimeRecordWithSystemd, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")

	var firmwareTs, loaderTs, initrdTs, userspaceTs, finishTs uint64
	properties := map[string]*uint64{
		"FirmwareTimestampMonotonic":  &firmwareTs,
		"LoaderTimestampMonotonic":    &loaderTs,
		"InitRDTimestampMonotonic":    &initrdTs,
		"UserspaceTimestampMonotonic": &userspaceTs,
		"FinishTimestampMonotonic":    &finishTs,
	}

	for propName, dest := range properties {
		var value dbus.Variant
		err := obj.Call("org.freedesktop.DBus.Properties.Get", 0,
			"org.freedesktop.systemd1.Manager", propName).Store(&value)
		if err != nil {
			continue
		}

		if val, ok := value.Value().(uint64); ok {
			*dest = val
		}
	}

	if finishTs == 0 {
		return nil, errors.New("bootup is not yet finished")
	}

	usec := func(us uint64) time.Duration {
		return time.Duration(us) * time.Microsecond
	}

	// Determine kernel_done_time
	var kernelDoneTime uint64
	if initrdTs > 0 {
		kernelDoneTime = initrdTs
	} else {
		kernelDoneTime = userspaceTs
	}

	record := &BootTimeRecordWithSystemd{}

	// Match systemd's calculation exactly
	if firmwareTs > 0 && loaderTs > 0 {
		record.Firmware = usec(firmwareTs - loaderTs)
	}

	if loaderTs > 0 {
		record.Loader = usec(loaderTs)
	}

	record.Kernel = usec(kernelDoneTime)

	if initrdTs > 0 && userspaceTs > 0 {
		record.Initrd = usec(userspaceTs - initrdTs)
	}

	if finishTs > 0 && userspaceTs > 0 {
		record.Userspace = usec(finishTs - userspaceTs)
	}

	if firmwareTs > 0 && finishTs > 0 {
		record.Total = usec(firmwareTs + finishTs)
	}

	return record, nil
}

func ParseSystemdAnalyzeTimeOutput(output string) (*BootTimeRecordWithSystemd, error) {
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return nil, errors.New("empty output")
	}

	line := lines[0]
	words := strings.Fields(line)

	var record BootTimeRecordWithSystemd
	var err error
	for idx, word := range words {
		switch {
		case strings.Contains(word, "(firmware)"):
			record.Firmware, err = time.ParseDuration(words[idx-1])
		case strings.Contains(word, "(loader)"):
			record.Loader, err = time.ParseDuration(words[idx-1])
		case strings.Contains(word, "(kernel)"):
			record.Kernel, err = time.ParseDuration(words[idx-1])
		case strings.Contains(word, "(initrd)"):
			record.Initrd, err = time.ParseDuration(words[idx-1])
		case strings.Contains(word, "(userspace)"):
			record.Userspace, err = time.ParseDuration(words[idx-1])
		case strings.Contains(word, "="):
			record.Total, err = time.ParseDuration(words[idx+1])
		}
		if err != nil {
			return nil, err
		}
	}
	return &record, nil
}

func RunAnalysis(fileName string) (*model.BootTimeRecord, error) {
	g := new(errgroup.Group)

	var recordSystemdAnalyze *BootTimeRecordWithSystemd
	g.Go(func() error {
		var err error
		recordSystemdAnalyze, err = RetrieveBootTimeWithSystemdAnalyze()
		if err != nil {
			return fmt.Errorf("retrieving boot time with systemd-analyze: %w", err)
		}
		return nil
	})

	var recordSystemdDbus *BootTimeRecordWithSystemd
	g.Go(func() error {
		var err error
		recordSystemdDbus, err = RetrieveBootTimeWithDbus()
		if err != nil {
			return fmt.Errorf("retrieving boot time with dbus property: %w", err)
		}
		return nil
	})

	var recordEFIVars *BootTimeRecordWithEFIVariables
	g.Go(func() error {
		var err error
		recordEFIVars, err = RetrieveBootTimeWithEFIVars()
		if err != nil {
			return fmt.Errorf("retrieving boot time with efi vars: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	values := map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
		model.BootTimeStageFirmware: {
			model.RetrievalMethodEFIVar:         recordEFIVars.Firmware,
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Firmware,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Firmware,
		},
		model.BootTimeStageLoader: {
			model.RetrievalMethodEFIVar:         recordEFIVars.Loader,
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Loader,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Loader,
		},
		model.BootTimeStageKernel: {
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Kernel,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Kernel,
		},
		model.BootTimeStageInitrd: {
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Initrd,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Initrd,
		},
		model.BootTimeStageUserspace: {
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Userspace,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Userspace,
		},
		model.BootTimeStageTotal: {
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Total,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Total,
		},
	}

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", fileName, err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	if err := enc.Encode(values); err != nil {
		return nil, fmt.Errorf("encoding analysis results to jsonl file: %w", err)
	}

	return &model.BootTimeRecord{
		Values: values,
	}, nil
}

func PrintRecordsAverage(fileName string, pretiffy bool) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", fileName, err)
	}
	defer file.Close()

	records, err := model.BootTimeRecordsFromFile(file)
	if err != nil {
		return fmt.Errorf("reading boot time records from file: %w", err)
	}

	btra := model.NewBootTimeAccumulator()
	for _, r := range records {
		btra.Add(r)
	}

	btr := btra.Average()

	if pretiffy {
		return printRecordsAveragePrettier(btr)
	}

	btrBytes, err := json.Marshal(&btr)
	if err != nil {
		return fmt.Errorf("marshalling averaged results to json: %w", err)
	}
	fmt.Printf("%s\n", string(btrBytes))

	return nil
}

func printRecordsAveragePrettier(btr *model.BootTimeRecord) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	rows := btr.ToTable()
	for _, row := range rows {
		for _, cell := range row {
			fmt.Fprint(w, cell, "\t")
		}
		fmt.Fprintln(w)
	}

	return w.Flush()
}
