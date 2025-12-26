package efi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const efivarsPath string = "/sys/firmware/efi/efivars"

type BootTimeRecord struct {
	Firmware time.Duration
	Loader   time.Duration
}

func RetrieveBootTime() (*BootTimeRecord, error) {
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

	return &BootTimeRecord{
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
	return data[4:], nil // skip the attributes part
}

func parseEFIMicroseconds(data []byte) (time.Duration, error) {
	if len(data)%2 != 0 {
		return 0, errors.New("invalid UTF-16 length")
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
