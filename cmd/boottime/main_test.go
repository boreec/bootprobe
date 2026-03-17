package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	tcs := map[string]struct {
		argv     []string
		validate func(t *testing.T, args *Args, flags *Flags, err error)
	}{
		"valid -R file.jsonl": {
			argv: []string{"-R", "file.jsonl"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.NoError(t, err)
				assert.True(t, flags.RunRetrieveBootTime)
				assert.False(t, flags.RunAggregate)
				assert.Equal(t, "file.jsonl", args.FileName)
			},
		},
		"valid -A file.jsonl": {
			argv: []string{"-A", "file.jsonl"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.NoError(t, err)
				assert.False(t, flags.RunRetrieveBootTime)
				assert.True(t, flags.RunAggregate)
				assert.Equal(t, "file.jsonl", args.FileName)
			},
		},
		"valid -A -p file.jsonl": {
			argv: []string{"-A", "-p", "file.jsonl"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.NoError(t, err)
				assert.True(t, flags.RunAggregate)
				assert.True(t, flags.Prettify)
				assert.Equal(t, "file.jsonl", args.FileName)
			},
		},
		"missing positional arg": {
			argv: []string{"-R"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "expected 1 arg")
			},
		},
		"wrong suffix": {
			argv: []string{"-R", "file.txt"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), ".jsonl suffix")
			},
		},
		"both -R and -A": {
			argv: []string{"-R", "-A", "file.jsonl"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "incompatible")
			},
		},
		"neither flag": {
			argv: []string{"file.jsonl"},
			validate: func(t *testing.T, args *Args, flags *Flags, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "required")
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var args Args
			var flags Flags
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			err := parseArgs(fs, tc.argv, &args, &flags)
			tc.validate(t, &args, &flags, err)
		})
	}
}
