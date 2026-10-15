package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/browser"
	"github.com/adegoodyer/mrm/internal/web"
)

var (
	flagWebOpen bool
	flagWebPort int
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Serve the web UI on 127.0.0.1 (overview, repo pages, activity, settings)",
	Long: `Serve the web UI on this machine only (127.0.0.1). The address printed
carries this run's access token; open it, or use --open. Ctrl+C stops it.`,
	Example: `  mrm web --open
  mrm web -w work --port 7421`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWeb(cmd.Context())
	},
}

func init() {
	webCmd.Flags().BoolVar(&flagWebOpen, "open", false, "open the web UI in the browser")
	webCmd.Flags().IntVar(&flagWebPort, "port", 0, "port (default web.port in the config, 7420)")
	rootCmd.AddCommand(webCmd)
}

func runWeb(ctx context.Context) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	port := flagWebPort
	if port == 0 {
		port = a.cfg.Web.Port
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("can't listen on 127.0.0.1:%d: %w (another mrm web? try --port)", port, err)
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv, err := web.New(ctx, web.Options{
		Config:     a.cfg,
		Engine:     a.eng,
		Workspaces: a.workspaces,
		Port:       port,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", port, srv.Token())
	fmt.Println("mrm web: " + url)
	fmt.Println("Ctrl+C to stop. The token is new each run; anyone with this address can sync your repos.")
	if flagWebOpen {
		if err := browser.Open(a.cfg.BrowserCmd, url); err != nil {
			fmt.Fprintln(os.Stderr, a.style.Yellow("warning: ")+err.Error())
		}
	}
	return srv.Serve(ln)
}
