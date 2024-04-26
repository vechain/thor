package main

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/urfave/cli.v1"
	"os"
	"testing"
)

func TestStringSliceFlag(t *testing.T) {
	// Setup
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name:  "tags",
			Usage: "list of tags",
		},
	}
	app.Action = func(c *cli.Context) error {
		tags := c.StringSlice("tags")
		assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
		return nil
	}

	// Simulate command line input
	os.Args = []string{"cmd", "--tags", "tag1", "tag2", "tag3"}

	// Run the application
	err := app.Run(os.Args)
	assert.Nil(t, err)
}
