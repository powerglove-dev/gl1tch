package cmd

import (
	"testing"
)

func TestConfigCmd_Registered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("config subcommand not registered in rootCmd")
	}
}

func TestConfigInitCmd_Registered(t *testing.T) {
	found := false
	for _, c := range configCmd.Commands() {
		if c.Use == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("config init subcommand not registered in configCmd")
	}
}

func TestDefaultLayoutYAML_NonEmpty(t *testing.T) {
	if defaultLayoutYAML == "" {
		t.Fatal("defaultLayoutYAML must not be empty")
	}
}

