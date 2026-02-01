// version command - displays build information
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("trading-bot v%s\n", Version)
		fmt.Printf("build time: %s\n", BuildTime)
		fmt.Printf("git commit: %s\n", GitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
