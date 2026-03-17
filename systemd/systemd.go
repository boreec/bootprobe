package systemd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// ErrParseAnalyzeCommandEmptyOutput is returned when the systemd-analyze time
// command returns an empty output.
var ErrParseAnalyzeCommandEmptyOutput = errors.New("command output is empty")

type BootTimeRecord struct {
	Firmware  time.Duration
	Loader    time.Duration
	Kernel    time.Duration
	Initrd    time.Duration
	Userspace time.Duration
	Total     time.Duration
}

func RetrieveBootTimeWithAnalyzeCommand() (*BootTimeRecord, error) {
	cmd := exec.Command("systemd-analyze", "time")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	btr, err := ParseAnalyzeCommandOutput(string(out))
	if err != nil {
		return nil, fmt.Errorf("parsing command output: %w", err)
	}

	return btr, nil
}

func RetrieveBootTimeWithDbus() (*BootTimeRecord, error) {
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

	return computeDbusBootTime(firmwareTs, loaderTs, initrdTs, userspaceTs, finishTs)
}

func computeDbusBootTime(firmwareTs, loaderTs, initrdTs, userspaceTs, finishTs uint64) (*BootTimeRecord, error) {
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

	record := &BootTimeRecord{}

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

// ParseAnalyzeCommandOutput parses the string output of the systemd-analyze time
// command and returns the duration.
func ParseAnalyzeCommandOutput(output string) (*BootTimeRecord, error) {
	lines := strings.Split(output, "\n")
	if output == "" || len(lines) == 0 {
		return nil, ErrParseAnalyzeCommandEmptyOutput
	}

	line := lines[0]
	words := strings.Fields(line)

	var record BootTimeRecord
	var err error
	for idx, word := range words {
		switch {
		case strings.Contains(word, "(firmware)"):
			record.Firmware, err = parseDuration(words[idx-1 : idx])
			if err != nil {
				err = fmt.Errorf("parsing firmware duration: %w", err)
			}
		case strings.Contains(word, "(loader)"):
			record.Loader, err = parseDuration(words[idx-1 : idx])
			if err != nil {
				err = fmt.Errorf("parsing loader duration: %w", err)
			}
		case strings.Contains(word, "(kernel)"):
			record.Kernel, err = parseDuration(words[idx-1 : idx])
			if err != nil {
				err = fmt.Errorf("parsing kernel duration: %w", err)
			}
		case strings.Contains(word, "(initrd)"):
			record.Initrd, err = parseDuration(words[idx-1 : idx])
			if err != nil {
				err = fmt.Errorf("parsing initrd duration: %w", err)
			}
		case strings.Contains(word, "(userspace)"):
			record.Userspace, err = parseDuration(words[idx-1 : idx])
			if err != nil {
				err = fmt.Errorf("parsing userspace duration: %w", err)
			}
		case strings.Contains(word, "="):
			record.Total, err = parseDuration(words[idx+1:])
			if err != nil {
				err = fmt.Errorf("parsing total duration: %w", err)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return &record, nil
}

func parseDuration(words []string) (time.Duration, error) {
	totalDuration := time.Duration(0)
	for _, w := range words {
		sanitizedWord := strings.ReplaceAll(w, "min", "m")
		d, err := time.ParseDuration(sanitizedWord)
		if err != nil {
			return totalDuration, fmt.Errorf("parsing time duration for word %s: %w", w, err)
		}
		totalDuration += d
	}
	return totalDuration, nil
}
