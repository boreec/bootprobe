package acpi

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadParsedSysfsAttribute(t *testing.T) {
	tcs := map[string]struct {
		content string
		missing bool
		validate func(t *testing.T, val uint64, err error)
	}{
		"valid numeric content": {
			content: "123456789",
			validate: func(t *testing.T, val uint64, err error) {
				require.NoError(t, err)
				assert.Equal(t, uint64(123456789), val)
			},
		},
		"non-numeric returns error": {
			content: "not-a-number",
			validate: func(t *testing.T, val uint64, err error) {
				require.Error(t, err)
			},
		},
		"missing file returns error": {
			missing: true,
			validate: func(t *testing.T, val uint64, err error) {
				require.Error(t, err)
			},
		},
		"content with trailing newline": {
			content: "42\n",
			validate: func(t *testing.T, val uint64, err error) {
				require.NoError(t, err)
				assert.Equal(t, uint64(42), val)
			},
		},
		"content with surrounding whitespace": {
			content: "  100  \n",
			validate: func(t *testing.T, val uint64, err error) {
				require.NoError(t, err)
				assert.Equal(t, uint64(100), val)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			overridePath(t, &pathFPDTBootDir, dir+"/")

			if !tc.missing {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "testattr"), []byte(tc.content), 0o644))
			}

			val, err := readParsedSysfsAttribute("testattr")
			tc.validate(t, val, err)
		})
	}
}

func TestRetrieveBootTimeWithSysfs(t *testing.T) {
	tcs := map[string]struct {
		setup    func(t *testing.T, dir string)
		validate func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"valid both attributes": {
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bootloader_launch_ns"), []byte("1000000000"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "exitbootservice_end_ns"), []byte("3000000000"), 0o644))
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1*time.Second, rec.Firmware)
				assert.Equal(t, 2*time.Second, rec.Loader)
			},
		},
		"missing one file": {
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bootloader_launch_ns"), []byte("1000000000"), 0o644))
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			overridePath(t, &pathFPDTBootDir, dir+"/")

			rec, err := retrieveBootTimeWithSysfs()
			tc.validate(t, rec, err)
		})
	}
}

func TestRetrieveBootTimeFromTablePointer(t *testing.T) {
	tcs := map[string]struct {
		data     []byte
		validate func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"table too short for header": {
			data: []byte{0x01, 0x02, 0x03},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no header")
			},
		},
		"table with no type-0 record": {
			data: buildFPDTTableWithPointer(TableHeaderFPDT{Type: 1, Length: 16, Revision: 1}, 0x1000),
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "pointer not found")
			},
		},
		"valid type-0 pointer record errors on dev mem read": {
			data: buildFPDTTableWithPointer(TableHeaderFPDT{Type: 0, Length: 16, Revision: 1}, 0xDEAD),
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tablePath := filepath.Join(dir, "FPDT")
			require.NoError(t, os.WriteFile(tablePath, tc.data, 0o644))

			overridePath(t, &pathFPDTTableFile, tablePath)
			overridePath(t, &pathDevMem, filepath.Join(dir, "mem"))

			rec, err := retrieveBootTimeFromTablePointer()
			tc.validate(t, rec, err)
		})
	}
}

func TestReadFPDTFromMemory(t *testing.T) {
	tcs := map[string]struct {
		buildFile func(t *testing.T) ([]byte, int64)
		validate  func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"valid type-2 record with OSLoaderLoadImageStart": {
			buildFile: func(t *testing.T) ([]byte, int64) {
				return buildFPDTMemory(TableRecordFPDT{
					Header:                  TableHeaderFPDT{Type: 2, Length: 48, Revision: 1},
					ResetEnd:                500000000,
					OSLoaderLoadImageStart:  1000000000,
					OSLoaderStartImageStart: 1500000000,
					ExitBootServicesEntry:   2000000000,
					ExitBootServicesExit:    2500000000,
				}), 0
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(1000000000)*time.Nanosecond, rec.Firmware)
				assert.Equal(t, time.Duration(1500000000)*time.Nanosecond, rec.Loader)
			},
		},
		"ResetEnd fallback when OSLoaderLoadImageStart is zero": {
			buildFile: func(t *testing.T) ([]byte, int64) {
				return buildFPDTMemory(TableRecordFPDT{
					Header:   TableHeaderFPDT{Type: 2, Length: 48, Revision: 1},
					ResetEnd: 500000000,
				}), 0
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(500000000)*time.Nanosecond, rec.Firmware)
				assert.Equal(t, time.Duration(0), rec.Loader)
			},
		},
		"wrong signature returns error": {
			buildFile: func(t *testing.T) ([]byte, int64) {
				data := buildFPDTMemory(TableRecordFPDT{
					Header: TableHeaderFPDT{Type: 2, Length: 48, Revision: 1},
				})
				copy(data[0:4], "XYZW")
				return data, 0
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not FPDT")
			},
		},
		"no type-2 record returns error": {
			buildFile: func(t *testing.T) ([]byte, int64) {
				hdr := makeFPDTTableHeader(36 + 16)
				ptrRec := TablePointerRecordFPDT{
					Header: TableHeaderFPDT{Type: 0, Length: 16, Revision: 1},
				}
				var buf bytes.Buffer
				buf.Write(hdr)
				binary.Write(&buf, binary.LittleEndian, ptrRec)
				return buf.Bytes(), 0
			},
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no boot performance record")
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			data, offset := tc.buildFile(t)
			dir := t.TempDir()
			memPath := filepath.Join(dir, "mem")
			require.NoError(t, os.WriteFile(memPath, data, 0o644))

			overridePath(t, &pathDevMem, memPath)

			rec, err := readFPDTFromMemory(offset)
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

func makeFPDTTableHeader(totalLength int) []byte {
	hdr := TableHeader{
		Length: uint32(totalLength),
	}
	copy(hdr.Signature[:], "FPDT")
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, hdr)
	return buf.Bytes()
}

func buildFPDTTableWithPointer(ptrHeader TableHeaderFPDT, addr uint64) []byte {
	hdr := makeFPDTTableHeader(36 + 16)
	rec := TablePointerRecordFPDT{
		Header:  ptrHeader,
		Address: addr,
	}
	var buf bytes.Buffer
	buf.Write(hdr)
	binary.Write(&buf, binary.LittleEndian, rec)
	return buf.Bytes()
}

func buildFPDTMemory(rec TableRecordFPDT) []byte {
	hdr := makeFPDTTableHeader(tableHeaderSize + int(rec.Header.Length))
	var buf bytes.Buffer
	buf.Write(hdr)
	binary.Write(&buf, binary.LittleEndian, rec)
	return buf.Bytes()
}
