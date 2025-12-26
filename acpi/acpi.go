// Package acpi is used to leverage the ACPI standard, especially by focusing
// on parsing the Firmware Performance Data Table to retrieve boot metrics.
package acpi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	tableHeaderSize   int    = 36
	pathDevMem        string = "/dev/mem"
	pathFPDTBootDir   string = "/sys/firmware/acpi/fpdt/boot/"
	pathFPDTTableFile string = "/sys/firmware/acpi/tables/FPDT"
)

// TableHeader is the standard header common to all ACPI tables (36 bytes).
type TableHeader struct {
	// Signature is a a 4-byte slice identifying the table ("ECDT", "FPDT", etc).
	Signature [4]byte
	// Length is the length of the entire table in bytes.
	Length uint32
	// Revision is the revision of the structure corresponding to the signature
	// field for this table. For the Firmware Performance Data Table conforming
	// to this revision of the specification, the revision is 1.
	Revision uint8
	// Checksum is used to check the table validity. The entire table, including
	// the checksum field, must add to zero to be considered valid.
	Checksum uint8
	// OEMID is an OEM-supplied string that identifies the OEM.
	OEMID [6]byte
	// OEMTableID is an OEM-supplied string that the OEM uses to identify this
	///particular data table.
	OEMTableID [8]byte
	// OEMRevision is an OEM-supplied revision number.
	OEMRevision uint32
	// CreatorID is the Vendor ID of the utility that created this table.
	CreatorID uint32
	// CreatorRevision is the revision of the utility that created this table.
	CreatorRevision uint32
}

// TableHeaderFPDT is the common header for FPDT records inside.
type TableHeaderFPDT struct {
	// Type depicts the format and contents of the performance record:
	//  - 0x0000: Firmware Basic Boot Performance Pointer Record.
	//  - 0x0001: S3 Performance Table Pointer Record.
	// 	- 0x0002 - 0x0FFF: Reserved for ACPI specification usage.
	//  - 0x1000 - 0x1FFF: Reserved for Platform Vendor usage.
	//  - 0x2000 - 0x2FFF: Reserved for Hardware Vendor usage.
	//  - 0x3000 - 0x3FFF: Reserved for platform firmware Vendor usage.
	//  - 0x4000 - 0xFFFF: Reserved for future use.
	Type uint16
	// Length of the performance record in bytes.
	Length uint8
	// Revision value is updated if the format of the record type is extended.
	// Any changes to a performance record layout must be backwards-compatible in
	// that all previously defined fields must be maintained if still applicable,
	// but newly defined fields allow the length of the performance record to be
	// increased. Previously defined record fields must not be redefined, but are
	// permitted to be deprecated.
	Revision uint8
}

// TablePointerRecordFPDT is a pointer to the FPDT values.
type TablePointerRecordFPDT struct {
	Header   TableHeaderFPDT
	Reserved uint32
	// Address is the physical address of the the FPDT table in memory.
	Address uint64
}

// TableRecordFPDT is the content of the FPDT table.
type TableRecordFPDT struct {
	// Header is the header of the table.
	Header TableHeaderFPDT
	// Reserved
	Reserved uint32
	// ResetEnd is the timer value logged at the beginning of firmware image
	// execution. This may not always be zero or near zero.
	ResetEnd uint64
	// OSLoaderLoadImageStart is the timer value logged just prior to loading the
	// OS boot loader into memory. For non-UEFI compatible boots, this field must
	// be zero
	OSLoaderLoadImageStart uint64
	// OSLoaderStartImageStart is timer value logged just prior to launching the
	// currently loaded OS boot loader image. For non-UEFI compatible boots, the
	// timer value logged will be just prior to the INT 19h handler invocation.
	OSLoaderStartImageStart uint64
	// ExitBootServicesEntry is the timer value logged at the point when the OS
	// loader calls the ExitBootServices function for UEFI compatible firmware.
	// For non-UEFI compatible boots, this field must be zero.
	ExitBootServicesEntry uint64
	// ExitBootServicesExit is the timer value logged at the point just prior to
	// the OS loader gaining control back from the ExitBootServices function for
	// UEFI compatible firmware. For non-UEFI compatible boots, this field must be
	// zero.
	ExitBootServicesExit uint64
}

// BootTimeRecord contains the duration of the boot time stages provided by
// the ACPI FPDT table.
type BootTimeRecord struct {
	Firmware time.Duration
	Loader   time.Duration
}

// RetrieveBootTime attempts to read boot times from Sysfs (Kernel 5.12+)
// and falls back to reading raw ACPI tables via /dev/mem.
func RetrieveBootTime() (*BootTimeRecord, error) {
	if times, err := retrieveBootTimeWithSysfs(); err == nil {
		return times, nil
	}

	return retrieveBootTimeFromTablePointer() // requires root access
}

// retrieveBootTimeWithSysfs reads parsed values from "/sys/firmware/acpi/fpdt/".
func retrieveBootTimeWithSysfs() (*BootTimeRecord, error) {
	launchNs, err := readParsedSysfsAttribute("bootloader_launch_ns")
	if err != nil {
		return nil, fmt.Errorf("reading attribute bootloader_launch_ns: %w", err)
	}

	exitNs, err := readParsedSysfsAttribute("exitbootservice_end_ns")
	if err != nil {
		return nil, fmt.Errorf("reading attribute exitbootservice_end_ns: %w", err)
	}

	return &BootTimeRecord{
		Firmware: time.Duration(launchNs) * time.Nanosecond,
		Loader:   time.Duration(exitNs-launchNs) * time.Nanosecond,
	}, nil
}

func readParsedSysfsAttribute(attribute string) (uint64, error) {
	path := filepath.Join(pathFPDTBootDir, attribute)
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return 0, fmt.Errorf("reading file %s: %w", path, err)
	}

	d, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing uint: %w", err)
	}

	return d, nil
}

func retrieveBootTimeFromTablePointer() (*BootTimeRecord, error) {
	data, err := os.ReadFile(filepath.Clean(pathFPDTTableFile))
	if err != nil {
		return nil, fmt.Errorf("read FPDT table file %s: %w", pathFPDTTableFile, err)
	}

	if len(data) < tableHeaderSize {
		return nil, errors.New("FPDT table have no header")
	}
	buf := bytes.NewReader(data[tableHeaderSize:]) // skip the header

	var fpdtAddress *uint64

	for buf.Len() > 0 {
		var sh TableHeaderFPDT
		headerBytes := make([]byte, 4)
		if _, err := buf.ReadAt(headerBytes, 0); err != nil {
			break
		}
		binary.Read(bytes.NewReader(headerBytes), binary.LittleEndian, &sh)

		if sh.Length == 0 {
			break // Avoid infinite loop
		}

		recordData := make([]byte, sh.Length)
		if _, err := buf.Read(recordData); err != nil {
			break
		}

		if sh.Type == 0 {
			var ptrRec TablePointerRecordFPDT
			if err := binary.Read(bytes.NewReader(recordData), binary.LittleEndian, &ptrRec); err == nil {
				fpdtAddress = &ptrRec.Address
				break
			}
		}
	}

	if fpdtAddress == nil {
		return nil, errors.New("FPDT pointer not found in FPDT table")
	}

	record, err := readFPDTFromMemory(int64(*fpdtAddress))
	if err != nil {
		return nil, fmt.Errorf("reading FPDT table from address %x: %w", fpdtAddress, err)
	}

	return record, nil
}

func readFPDTFromMemory(physAddr int64) (*BootTimeRecord, error) {
	mem, err := os.Open(filepath.Clean(pathDevMem))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", pathDevMem, err)
	}
	defer mem.Close()

	headerBuf := make([]byte, tableHeaderSize)
	if _, err := mem.ReadAt(headerBuf, physAddr); err != nil {
		return nil, fmt.Errorf("reading ACPI table header: %w", err)
	}

	var hdr TableHeader
	if err := binary.Read(bytes.NewReader(headerBuf), binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("parsing ACPI table header: %w", err)
	}

	if string(hdr.Signature[:]) != "FPDT" {
		return nil, fmt.Errorf("table signature memory is not FPDT, but %s", hdr.Signature)
	}

	tableData := make([]byte, hdr.Length)
	if _, err := mem.ReadAt(tableData, physAddr); err != nil {
		return nil, fmt.Errorf("reading full table: %w", err)
	}

	offset := tableHeaderSize // skip header
	for offset < int(hdr.Length) {
		r := bytes.NewReader(tableData[offset:])
		var sh TableHeaderFPDT
		if err := binary.Read(r, binary.LittleEndian, &sh); err != nil {
			break
		}

		if sh.Length == 0 {
			break
		}

		if sh.Type == 2 {
			var rec TableRecordFPDT
			r = bytes.NewReader(tableData[offset:]) // reset reader to the record start
			if err := binary.Read(r, binary.LittleEndian, &rec); err != nil {
				return nil, fmt.Errorf("parsing boot record: %w", err)
			}

			result := &BootTimeRecord{}

			// Firmware = Time until Loader Starts
			if rec.OSLoaderLoadImageStart > 0 {
				result.Firmware = time.Duration(rec.OSLoaderLoadImageStart) * time.Nanosecond
			} else if rec.ResetEnd > 0 {
				result.Firmware = time.Duration(rec.ResetEnd) * time.Nanosecond
			}

			// Loader = Time from Loader Start until ExitBootServices (Kernel handover)
			if rec.ExitBootServicesExit > 0 && rec.OSLoaderLoadImageStart > 0 {
				if rec.ExitBootServicesExit > rec.OSLoaderLoadImageStart {
					result.Loader = time.Duration(rec.ExitBootServicesExit-rec.OSLoaderLoadImageStart) * time.Nanosecond
				}
			}

			return result, nil
		}

		offset += int(sh.Length)
	}

	return nil, errors.New("no boot performance record found in FPDT")
}
