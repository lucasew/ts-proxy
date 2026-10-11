package main

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type configCmd struct{}

func (configCmd) Description() string {
	return "Print the resolved configuration."
}

func (c *configCmd) Run(ctx context.Context) error {
	flags, err := flagsFrom(ctx)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(flags)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
