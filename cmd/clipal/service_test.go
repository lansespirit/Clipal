package main

import (
	"reflect"
	"testing"
)

const exampleWindowsConfigDir = `C:\Users\example\.clipal`

func TestSplitServiceArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantAct  string
		wantFlag []string
		wantErr  bool
	}{
		{
			name:     "FlagsAfterAction",
			args:     []string{"install", "--dry-run"},
			wantAct:  "install",
			wantFlag: []string{"--dry-run"},
		},
		{
			name:     "FlagsBeforeAction",
			args:     []string{"--dry-run", "install"},
			wantAct:  "install",
			wantFlag: []string{"--dry-run"},
		},
		{
			name:     "ValueFlagAfterAction",
			args:     []string{"install", "--config-dir", exampleWindowsConfigDir},
			wantAct:  "install",
			wantFlag: []string{"--config-dir", exampleWindowsConfigDir},
		},
		{
			name:     "ValueFlagBeforeAction",
			args:     []string{"--config-dir", exampleWindowsConfigDir, "install"},
			wantAct:  "install",
			wantFlag: []string{"--config-dir", exampleWindowsConfigDir},
		},
		{
			name:     "InlineValueFlag",
			args:     []string{"install", "--config-dir=" + exampleWindowsConfigDir},
			wantAct:  "install",
			wantFlag: []string{"--config-dir=" + exampleWindowsConfigDir},
		},
		{
			name:     "Terminator",
			args:     []string{"--dry-run", "--", "install"},
			wantAct:  "install",
			wantFlag: []string{"--dry-run"},
		},
		{
			name:    "MissingAction",
			args:    []string{"--dry-run"},
			wantErr: true,
		},
		{
			name:    "ExtraArg",
			args:    []string{"install", "extra"},
			wantErr: true,
		},
		{
			name:     "ValueCanStartWithDash",
			args:     []string{"install", "--config-dir", "-weird"},
			wantAct:  "install",
			wantFlag: []string{"--config-dir", "-weird"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			act, flags, err := splitServiceArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if act != tt.wantAct {
				t.Fatalf("action=%q want=%q", act, tt.wantAct)
			}
			if !reflect.DeepEqual(flags, tt.wantFlag) {
				t.Fatalf("flags=%#v want=%#v", flags, tt.wantFlag)
			}
		})
	}
}
