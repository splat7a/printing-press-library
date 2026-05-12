package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/marketing/meta-ads/internal/meta"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCommand().Execute()
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "meta-ads-pp-cli",
		Short:         "Meta Ads Printing Press CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newSnapshotCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newAgentContextCommand())
	return root
}

func newSnapshotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Pull raw Meta Ads snapshots",
	}
	cmd.AddCommand(newSnapshotPullCommand())
	return cmd
}

func newSnapshotPullCommand() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull SabeenManekia-compatible raw Meta JSON files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(outDir) == "" {
				return fmt.Errorf("--out is required")
			}
			cfg := ConfigFromEnv()
			if err := cfg.Validate(); err != nil {
				return err
			}
			client := &http.Client{Timeout: 60 * time.Second}
			puller := meta.NewPuller(cfg, client, cmd.OutOrStdout())
			return puller.PullSnapshot(cmd.Context(), outDir)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "directory for raw Meta JSON files")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func newDoctorCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Meta Ads CLI environment without printing secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := ConfigFromEnv()
			status := map[string]any{
				"ok":                     cfg.Validate() == nil,
				"meta_access_token":      present(cfg.AccessToken),
				"meta_ad_account_id":     present(cfg.AdAccountID),
				"meta_business_id":       present(cfg.BusinessID),
				"meta_graph_api_version": cfg.GraphAPIVersion,
				"meta_graph_base_url":    cfg.GraphBaseURL,
			}
			if err := cfg.Validate(); err != nil {
				status["error"] = err.Error()
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}
			for _, key := range []string{"meta_access_token", "meta_ad_account_id", "meta_business_id"} {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", key, status[key])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "meta_graph_api_version: %s\n", cfg.GraphAPIVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "meta_graph_base_url: %s\n", cfg.GraphBaseURL)
			if err, ok := status["error"].(string); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "error: %s\n", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func newAgentContextCommand() *cobra.Command {
	var pretty bool
	cmd := &cobra.Command{
		Use:   "agent-context",
		Short: "Describe safe automation contract for agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := map[string]any{
				"name":        "meta-ads-pp-cli",
				"description": "Meta Ads raw snapshot puller for Printing Press workflows",
				"credentials": []string{"META_ACCESS_TOKEN", "META_AD_ACCOUNT_ID", "META_BUSINESS_ID"},
				"optional_env": []string{
					"META_GRAPH_API_VERSION (default v21.0)",
					"META_GRAPH_BASE_URL (test override)",
				},
				"primary_command": []string{"meta-ads-pp-cli", "snapshot", "pull", "--out", "raw/meta"},
				"raw_files":       meta.RawFilenames(),
				"secret_policy":   "read Meta credentials from environment only; never pass access tokens as CLI arguments; page access tokens are not requested or persisted",
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			if pretty {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(ctx)
		},
	}
	cmd.Flags().BoolVar(&pretty, "pretty", false, "pretty-print JSON")
	return cmd
}

func ConfigFromEnv() meta.Config {
	cfg := meta.Config{
		AccessToken:     os.Getenv("META_ACCESS_TOKEN"),
		AdAccountID:     os.Getenv("META_AD_ACCOUNT_ID"),
		BusinessID:      os.Getenv("META_BUSINESS_ID"),
		GraphAPIVersion: os.Getenv("META_GRAPH_API_VERSION"),
		GraphBaseURL:    os.Getenv("META_GRAPH_BASE_URL"),
	}
	if cfg.GraphAPIVersion == "" {
		cfg.GraphAPIVersion = meta.DefaultGraphAPIVersion
	}
	if cfg.GraphBaseURL == "" {
		cfg.GraphBaseURL = meta.DefaultGraphBaseURL
	}
	return cfg
}

func present(value string) string {
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return "set"
}

func ExecuteContext(ctx context.Context) error {
	cmd := NewRootCommand()
	cmd.SetContext(ctx)
	return cmd.Execute()
}
