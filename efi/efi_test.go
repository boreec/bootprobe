package efi

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEFIMicroseconds(t *testing.T) {
	tcs := map[string]struct {
		data     []byte
		validate func(t *testing.T, d time.Duration, err error)
	}{
		"valid UTF-16LE": {
			data: utf16LEEncode("12345"),
			validate: func(t *testing.T, d time.Duration, err error) {
				require.NoError(t, err)
				assert.Equal(t, 12345*time.Microsecond, d)
			},
		},
		"NUL-terminated string": {
			data: append(utf16LEEncode("99"), 0x00, 0x00),
			validate: func(t *testing.T, d time.Duration, err error) {
				require.NoError(t, err)
				assert.Equal(t, 99*time.Microsecond, d)
			},
		},
		"odd-length data returns error": {
			data: []byte{0x31, 0x00, 0x32},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "UTF-16")
			},
		},
		"empty data": {
			data: []byte{},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.Error(t, err)
			},
		},
		"non-numeric content returns error": {
			data: utf16LEEncode("abc"),
			validate: func(t *testing.T, d time.Duration, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := parseEFIMicroseconds(tc.data)
			tc.validate(t, d, err)
		})
	}
}

func TestReadEFIVarValue(t *testing.T) {
	tcs := map[string]struct {
		content []byte
		missing bool
		validate func(t *testing.T, data []byte, err error)
	}{
		"valid data with 4-byte header": {
			content: append([]byte{0x07, 0x00, 0x00, 0x00}, utf16LEEncode("500")...),
			validate: func(t *testing.T, data []byte, err error) {
				require.NoError(t, err)
				assert.Equal(t, utf16LEEncode("500"), data)
			},
		},
		"file shorter than 4 bytes": {
			content: []byte{0x01, 0x02},
			validate: func(t *testing.T, data []byte, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "too short")
			},
		},
		"missing file returns error": {
			missing: true,
			validate: func(t *testing.T, data []byte, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "efivar")

			if !tc.missing {
				require.NoError(t, os.WriteFile(path, tc.content, 0o644))
			}

			data, err := readEFIVarValue(path)
			tc.validate(t, data, err)
		})
	}
}

func TestRetrieveBootTime(t *testing.T) {
	tcs := map[string]struct {
		setup    func(t *testing.T, dir string)
		validate func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"valid init and exec vars": {
			setup: func(t *testing.T, dir string) {
				writeEFIVar(t, dir, "LoaderTimeInitUSec-abc", "1000")
				writeEFIVar(t, dir, "LoaderTimeExecUSec-abc", "3000")
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1000*time.Microsecond, rec.Firmware)
				assert.Equal(t, 2000*time.Microsecond, rec.Loader)
			},
		},
		"missing one variable": {
			setup: func(t *testing.T, dir string) {
				writeEFIVar(t, dir, "LoaderTimeInitUSec-abc", "1000")
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			},
		},
		"execTime less than initTime": {
			setup: func(t *testing.T, dir string) {
				writeEFIVar(t, dir, "LoaderTimeInitUSec-abc", "5000")
				writeEFIVar(t, dir, "LoaderTimeExecUSec-abc", "1000")
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "exec time < init time")
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)

			overridePath(t, &efivarsPath, dir)

			rec, err := RetrieveBootTime()
			tc.validate(t, rec, err)
		})
	}
}

func overridePath(t *testing.T, target *string, value string) {
	t.Helper()
	orig := *target
	*target = value
	t.Cleanup(func() { *target = orig })
}

func utf16LEEncode(s string) []byte {
	buf := make([]byte, len(s)*2)
	for i, r := range s {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(r))
	}
	return buf
}

func writeEFIVar(t *testing.T, dir, name, value string) {
	t.Helper()
	payload := utf16LEEncode(value)
	data := append([]byte{0x07, 0x00, 0x00, 0x00}, payload...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
}
