package systemd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAnalyzeCommandOutput(t *testing.T) {
	tcs := map[string]struct {
		input    string
		validate func(t *testing.T, btr *BootTimeRecord, err error)
	}{
		"parse valid input successfully": {
			input: `Startup finished in 1.897s (firmware) + 1.715s (loader) + 718ms (kernel) + 2.049s (initrd) + 13.275s (userspace) = 19.656s
graphical.target reached after 13.270s in userspace.`,
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.NoError(t, err)
				require.NotNil(t, btr)
				assert.Equal(t, time.Duration(1897)*time.Millisecond, btr.Firmware)
				assert.Equal(t, time.Duration(1715)*time.Millisecond, btr.Loader)
				assert.Equal(t, time.Duration(718)*time.Millisecond, btr.Kernel)
				assert.Equal(t, time.Duration(2049)*time.Millisecond, btr.Initrd)
				assert.Equal(t, time.Duration(13275)*time.Millisecond, btr.Userspace)
				assert.Equal(t, time.Duration(19656)*time.Millisecond, btr.Total)
			},
		},
		"parse valid input successfully for long boot": {
			input: `Startup finished in 1.734s (firmware) + 3.698s (loader) + 716ms (kernel) + 1.722s (initrd) + 58.126s (userspace) = 1min 5.998s
graphical.target reached after 58.126s in userspace.`,
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.NoError(t, err)
				require.NotNil(t, btr)
				assert.Equal(t, time.Duration(1734)*time.Millisecond, btr.Firmware)
				assert.Equal(t, time.Duration(3698)*time.Millisecond, btr.Loader)
				assert.Equal(t, time.Duration(716)*time.Millisecond, btr.Kernel)
				assert.Equal(t, time.Duration(1722)*time.Millisecond, btr.Initrd)
				assert.Equal(t, time.Duration(58126)*time.Millisecond, btr.Userspace)
				assert.Equal(t, time.Duration(65998)*time.Millisecond, btr.Total)
			},
		},
		"parse empty input returns error": {
			input: "",
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.ErrorIs(t, err, ErrParseAnalyzeCommandEmptyOutput)
				require.Nil(t, btr)
			},
		},
		"parse input with bad duration returns error": {
			input: `Startup finished in potatoes (firmware) + potatoes (loader) + potatoesms (kernel) + 2.049potatoes (initrd) + 13.275s (userspace) = 19.656s
graphical.target reached after 13.270s in userspace.`,
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.Error(t, err)
				require.Nil(t, btr)
			},
		},
		"output without initrd token": {
			input: `Startup finished in 1.897s (firmware) + 1.715s (loader) + 718ms (kernel) + 13.275s (userspace) = 17.605s
graphical.target reached after 13.270s in userspace.`,
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.NoError(t, err)
				require.NotNil(t, btr)
				assert.Equal(t, time.Duration(0), btr.Initrd)
				assert.Equal(t, time.Duration(1897)*time.Millisecond, btr.Firmware)
				assert.Equal(t, time.Duration(718)*time.Millisecond, btr.Kernel)
			},
		},
		"output without firmware and loader": {
			input: `Startup finished in 718ms (kernel) + 2.049s (initrd) + 13.275s (userspace) = 16.042s
graphical.target reached after 13.270s in userspace.`,
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.NoError(t, err)
				require.NotNil(t, btr)
				assert.Equal(t, time.Duration(0), btr.Firmware)
				assert.Equal(t, time.Duration(0), btr.Loader)
				assert.Equal(t, time.Duration(718)*time.Millisecond, btr.Kernel)
			},
		},
		"whitespace-only input returns zeroed record": {
			input: "   \n  \n",
			validate: func(t *testing.T, btr *BootTimeRecord, err error) {
				require.NoError(t, err)
				require.NotNil(t, btr)
				assert.Equal(t, time.Duration(0), btr.Firmware)
				assert.Equal(t, time.Duration(0), btr.Total)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			btr, err := ParseAnalyzeCommandOutput(tc.input)
			tc.validate(t, btr, err)
		})
	}
}

func TestParseDuration(t *testing.T) {
	tcs := map[string]struct {
		words    []string
		validate func(t *testing.T, d time.Duration, err error)
	}{
		"empty slice returns 0": {
			words: []string{},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(0), d)
			},
		},
		"single word seconds": {
			words: []string{"1.5s"},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1500*time.Millisecond, d)
			},
		},
		"multiple words with min": {
			words: []string{"1min", "5.998s"},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(65998)*time.Millisecond, d)
			},
		},
		"invalid word returns error": {
			words: []string{"potatoes"},
			validate: func(t *testing.T, d time.Duration, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := parseDuration(tc.words)
			tc.validate(t, d, err)
		})
	}
}

func TestComputeDbusBootTime(t *testing.T) {
	tcs := map[string]struct {
		firmwareTs  uint64
		loaderTs    uint64
		initrdTs    uint64
		userspaceTs uint64
		finishTs    uint64
		validate    func(t *testing.T, rec *BootTimeRecord, err error)
	}{
		"all timestamps non-zero": {
			firmwareTs:  5000000,
			loaderTs:    3000000,
			initrdTs:    1000000,
			userspaceTs: 2000000,
			finishTs:    4000000,
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2*time.Second, rec.Firmware)
				assert.Equal(t, 3*time.Second, rec.Loader)
				assert.Equal(t, 1*time.Second, rec.Kernel)
				assert.Equal(t, 1*time.Second, rec.Initrd)
				assert.Equal(t, 2*time.Second, rec.Userspace)
				assert.Equal(t, 9*time.Second, rec.Total)
			},
		},
		"initrdTs zero — kernelDoneTime falls back to userspaceTs": {
			firmwareTs:  5000000,
			loaderTs:    3000000,
			initrdTs:    0,
			userspaceTs: 2000000,
			finishTs:    4000000,
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2*time.Second, rec.Kernel)
				assert.Equal(t, time.Duration(0), rec.Initrd)
			},
		},
		"firmwareTs zero — no firmware or total": {
			firmwareTs:  0,
			loaderTs:    3000000,
			initrdTs:    1000000,
			userspaceTs: 2000000,
			finishTs:    4000000,
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(0), rec.Firmware)
				// Loader still set because loaderTs > 0
				assert.Equal(t, 3*time.Second, rec.Loader)
				// Total = 0 because firmwareTs == 0
				assert.Equal(t, time.Duration(0), rec.Total)
			},
		},
		"finishTs zero returns error": {
			firmwareTs:  5000000,
			loaderTs:    3000000,
			initrdTs:    1000000,
			userspaceTs: 2000000,
			finishTs:    0,
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not yet finished")
			},
		},
		"loaderTs zero — no firmware or loader": {
			firmwareTs:  5000000,
			loaderTs:    0,
			initrdTs:    1000000,
			userspaceTs: 2000000,
			finishTs:    4000000,
			validate: func(t *testing.T, rec *BootTimeRecord, err error) {
				require.NoError(t, err)
				assert.Equal(t, time.Duration(0), rec.Firmware)
				assert.Equal(t, time.Duration(0), rec.Loader)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, err := computeDbusBootTime(tc.firmwareTs, tc.loaderTs, tc.initrdTs, tc.userspaceTs, tc.finishTs)
			tc.validate(t, rec, err)
		})
	}
}
