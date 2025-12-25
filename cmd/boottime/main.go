package main

import (
	"errors"
	"flag"
	"strings"

	"github.com/boreec/boottime/exec"
)

func main() {
	args, flags, err := parseArgs()
	if err != nil {
		panic(err.Error())
	}

	if flags.RunRetrieveBootTime {
		if _, err := exec.RunAnalysis(args.FileName); err != nil {
			panic(err.Error())
		}
	}

	if flags.RunAggregate {
		if err := exec.PrintRecordsAverage(args.FileName, flags.Prettify); err != nil {
			panic(err.Error())
		}
	}
}

type Flags struct {
	RunRetrieveBootTime bool
	RunAggregate        bool
	Prettify            bool
}

type Args struct {
	FileName string
}

func parseArgs() (*Args, *Flags, error) {
	var flags Flags
	flag.BoolVar(&flags.RunRetrieveBootTime, "R", false, "retrieve boot time")
	flag.BoolVar(&flags.RunRetrieveBootTime, "retrieve-boot-time", false, "retrieve boot time")

	flag.BoolVar(&flags.RunAggregate, "A", false, "average boot time records")
	flag.BoolVar(&flags.RunAggregate, "average-boot-records", false, "average boot time records")

	flag.BoolVar(&flags.Prettify, "p", false, "prettify results")
	flag.BoolVar(&flags.Prettify, "prettify", false, "prettify results")
	flag.Parse()

	argsUnparsed := flag.Args()
	if len(argsUnparsed) == 0 {
		return nil, nil, errors.New("expected 1 arg for jsonl file, found 0")
	}
	var args Args
	args.FileName = argsUnparsed[0]

	if !strings.HasSuffix(args.FileName, ".jsonl") {
		return nil, nil, errors.New("argument should be a file name with .jsonl suffix")
	}

	if flags.RunAggregate && flags.RunRetrieveBootTime {
		return nil, nil, errors.New("flags -A and -R are incompatible")
	}

	if !flags.RunAggregate && !flags.RunRetrieveBootTime {
		return nil, nil, errors.New("flags -A or -R required")
	}

	return &args, &flags, nil
}
