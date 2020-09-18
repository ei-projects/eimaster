package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var version string = "master" // Replaced by linker, see Makefile
var log = logrus.New()

func main() {
	log.SetLevel(logrus.DebugLevel)

	var cmdVersion = &cobra.Command{
		Use:   "version",
		Short: "Print version of eimaster",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			println(version)
		},
	}

	var cmdClient = &cobra.Command{
		Use:   "client",
		Short: "Client commands",
	}
	for _, cmd := range clientCmds {
		cmdClient.AddCommand(cmd)
	}

	var cmdServer = &cobra.Command{
		Use:   "server",
		Short: "Server commands",
	}
	for _, cmd := range serverCmds {
		cmdServer.AddCommand(cmd)
	}

	var rootCmd = &cobra.Command{
		Use:   "eimaster",
		Short: "EI master is a server which hosts list of running game servers",
	}
	rootCmd.AddCommand(cmdClient, cmdServer, cmdVersion)
	rootCmd.Execute()
}
