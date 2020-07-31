package cmd

import (
	log "github.com/sirupsen/logrus"

	"github.com/empreinte-digitale/qproxy/pkg/qproxy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "qproxy",
	Short: "Start QProxy",
	Run: func(cmd *cobra.Command, args []string) {
		qproxy.Start()
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Unable to start QProxy")
	}
}

func init() {
	v := viper.GetViper()
	qproxy.SetCommandFlags(rootCmd.Flags(), v)
	cobra.OnInitialize(func() {
		if err := qproxy.InitConfig(v); err != nil {
			log.WithFields(log.Fields{"error": err}).Fatal("Unable to start QProxy")
		}
	})
}
