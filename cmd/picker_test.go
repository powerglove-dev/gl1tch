package cmd

import (
	"testing"
)

func TestPickerCmd_Registered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "picker" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("picker subcommand not registered in rootCmd")
	}
}

