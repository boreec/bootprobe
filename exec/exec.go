package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/boreec/boottime/acpi"
	"github.com/boreec/boottime/efi"
	"github.com/boreec/boottime/model"
	"github.com/boreec/boottime/systemd"
	"golang.org/x/sync/errgroup"
)

func RetrieveBootTimes(fileName string) error {
	g := new(errgroup.Group)

	var recordSystemdAnalyze *systemd.BootTimeRecord
	g.Go(func() error {
		var err error
		recordSystemdAnalyze, err = systemd.RetrieveBootTimeWithAnalyzeCommand()
		if err != nil {
			return fmt.Errorf("retrieving boot time with systemd-analyze: %w", err)
		}
		return nil
	})

	var recordSystemdDbus *systemd.BootTimeRecord
	g.Go(func() error {
		var err error
		recordSystemdDbus, err = systemd.RetrieveBootTimeWithDbus()
		if err != nil {
			return fmt.Errorf("retrieving boot time with dbus property: %w", err)
		}
		return nil
	})

	var recordEFIVars *efi.BootTimeRecord
	g.Go(func() error {
		var err error
		recordEFIVars, err = efi.RetrieveBootTime()
		if err != nil {
			return fmt.Errorf("retrieving boot time with efi vars: %w", err)
		}
		return nil
	})

	var recordACPIFPDT *acpi.BootTimeRecord
	g.Go(func() error {
		var err error
		recordACPIFPDT, err = acpi.RetrieveBootTime()
		if err != nil {
			return fmt.Errorf("reading acpi fpdt table: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	values := map[model.BootTimeStage]map[model.RetrievalMethod]time.Duration{
		model.BootTimeStageFirmware: {
			model.RetrievalMethodACPIFPDT:       recordACPIFPDT.Firmware,
			model.RetrievalMethodEFIVar:         recordEFIVars.Firmware,
			model.RetrievalMethodSystemdAnalyze: recordSystemdAnalyze.Firmware,
			model.RetrievalMethodSystemdDBUS:    recordSystemdDbus.Firmware,
		},
		model.BootTimeStageLoader: {
			model.RetrievalMethodACPIFPDT:       recordACPIFPDT.Loader,
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
		return fmt.Errorf("opening file %s: %w", fileName, err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	if err := enc.Encode(values); err != nil {
		return fmt.Errorf("encoding analysis results to jsonl file: %w", err)
	}

	return nil
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
		fmt.Printf("Boot time average for %d records.\n", len(records))
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
