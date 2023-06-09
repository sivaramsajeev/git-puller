package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/olekukonko/tablewriter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type GitPullCommand struct {
	rootCmd    *cobra.Command
	debug      bool
	logLevel   string
	logger     *logrus.Logger
	summary    [][]string
	wg         sync.WaitGroup
	mu         sync.Mutex
}

func NewGitPullCommand() *GitPullCommand {
	g := &GitPullCommand{
		logger:  logrus.New(),
		summary: [][]string{},
	}

	g.rootCmd = &cobra.Command{
		Use:   "gitpull",
		Short: "Traverse directories and perform git pull",
		Args:  cobra.ExactArgs(1),
		Run:   g.run,
	}

	g.rootCmd.PersistentFlags().BoolVar(&g.debug, "debug", false, "Enable debug logging")
	g.rootCmd.PersistentFlags().StringVar(&g.logLevel, "log-level", "error", "Logging level (options: debug, info, warning, error, fatal, panic)")
	g.rootCmd.ParseFlags(os.Args)

	g.setupLogger()

	return g
}

func (g *GitPullCommand) setupLogger() {
	g.logger.SetOutput(os.Stdout)
	g.logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
	})

	level, err := logrus.ParseLevel(g.logLevel)
	if err != nil {
		fmt.Printf("Invalid log level: %v\n", err)
		os.Exit(1)
	}

	if g.debug {
		level = logrus.DebugLevel
	}

	g.logger.SetLevel(level)
}

func (g *GitPullCommand) run(cmd *cobra.Command, args []string) {
	dir := args[0]

	err := filepath.Walk(dir, g.visit)
	if err != nil {
		g.logger.Errorf("Error: %v", err)
	}

	g.wait()

	g.printSummary()
}

func (g *GitPullCommand) visit(path string, info os.FileInfo, err error) error {
	if err != nil {
		g.logger.Errorf("Error accessing path: %v", err)
		return nil
	}

	if info.IsDir() && info.Name() == ".git" {
		repoDir := filepath.Dir(path)
		g.wg.Add(1)
		go g.pullRepository(repoDir)

		// Skip traversing subdirectories within repositories
		return filepath.SkipDir
	}

	return nil
}

func (g *GitPullCommand) pullRepository(dir string) {
	defer g.wg.Done()

	remote, status := g.getGitStatus(dir)
	g.mu.Lock()
	g.summary = append(g.summary, []string{dir, remote, status})
	g.mu.Unlock()

	// Perform git pull
	g.logger.Infof("Performing git pull for repository: %s", dir)
	cmd := exec.Command("git", "-C", dir, "pull")
	err := cmd.Run()
	if err != nil {
		g.logger.Errorf("Error executing git pull: %v", err)
		g.mu.Lock()
		g.updateStatus(dir, "Failed")
		g.mu.Unlock()
	} else {
		g.mu.Lock()
		g.updateStatus(dir, "Success")
		g.mu.Unlock()
	}
}

func (g *GitPullCommand) updateStatus(dir, status string) {
	for i, row := range g.summary {
		if row[0] == dir {
			g.summary[i][2] = status
			break
		}
	}
}

func (g *GitPullCommand) getGitStatus(dir string) (string, string) {
	cmd := exec.Command("git", "-C", dir, "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		g.logger.Errorf("Error executing git remote: %v", err)
		return "", "Unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 1 {
		return "", "Unknown"
	}

	remoteLine := strings.TrimSpace(lines[0])
	remoteParts := strings.Fields(remoteLine)
	if len(remoteParts) != 3 {
		return "", "Unknown"
	}

	remote := remoteParts[1]
	return remote, "Pending"
}

func (g *GitPullCommand) wait() {
	g.wg.Wait()
}

func (g *GitPullCommand) printSummary() {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Directory", "Remote", "Status"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetAutoWrapText(false)

	for _, row := range g.summary {
		table.Append(row)
	}

	table.Render()
}

func main() {
	cmd := NewGitPullCommand()
	if err := cmd.rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
