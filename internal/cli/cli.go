package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Description string
	Usage       string
	Run         func(args []string) error
	Subcommands map[string]*Command
}

// CLI manages the command-line interface
type CLI struct {
	rootCmd *Command
	paths   *config.Paths
}

// New creates a new CLI
func New() (*CLI, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}

	cli := &CLI{
		paths: paths,
		rootCmd: &Command{
			Name:        "multiclaude",
			Description: "repo-centric orchestrator for Claude Code",
			Subcommands: make(map[string]*Command),
		},
	}

	cli.registerCommands()
	return cli, nil
}

// Execute executes the CLI with the given arguments
func (c *CLI) Execute(args []string) error {
	if len(args) == 0 {
		return c.showHelp()
	}

	return c.executeCommand(c.rootCmd, args)
}

// executeCommand recursively executes commands and subcommands
func (c *CLI) executeCommand(cmd *Command, args []string) error {
	if len(args) == 0 {
		if cmd.Run != nil {
			return cmd.Run([]string{})
		}
		return c.showCommandHelp(cmd)
	}

	// Check for subcommands
	if subcmd, exists := cmd.Subcommands[args[0]]; exists {
		return c.executeCommand(subcmd, args[1:])
	}

	// No subcommand found, run this command with args
	if cmd.Run != nil {
		return cmd.Run(args)
	}

	return fmt.Errorf("unknown command: %s", args[0])
}

// showHelp shows the main help message
func (c *CLI) showHelp() error {
	fmt.Println("multiclaude - repo-centric orchestrator for Claude Code")
	fmt.Println()
	fmt.Println("Usage: multiclaude <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")

	for name, cmd := range c.rootCmd.Subcommands {
		fmt.Printf("  %-15s %s\n", name, cmd.Description)
	}

	fmt.Println()
	fmt.Println("Use 'multiclaude <command> --help' for more information about a command.")
	return nil
}

// showCommandHelp shows help for a specific command
func (c *CLI) showCommandHelp(cmd *Command) error {
	fmt.Printf("%s - %s\n", cmd.Name, cmd.Description)
	fmt.Println()
	if cmd.Usage != "" {
		fmt.Printf("Usage: %s\n", cmd.Usage)
		fmt.Println()
	}

	if len(cmd.Subcommands) > 0 {
		fmt.Println("Subcommands:")
		for name, subcmd := range cmd.Subcommands {
			fmt.Printf("  %-15s %s\n", name, subcmd.Description)
		}
		fmt.Println()
	}

	return nil
}

// registerCommands registers all CLI commands
func (c *CLI) registerCommands() {
	// Daemon commands
	c.rootCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the multiclaude daemon",
		Usage:       "multiclaude start",
		Run:         c.startDaemon,
	}

	daemonCmd := &Command{
		Name:        "daemon",
		Description: "Manage the multiclaude daemon",
		Subcommands: make(map[string]*Command),
	}

	daemonCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the daemon",
		Run:         c.startDaemon,
	}

	daemonCmd.Subcommands["stop"] = &Command{
		Name:        "stop",
		Description: "Stop the daemon",
		Run:         c.stopDaemon,
	}

	daemonCmd.Subcommands["status"] = &Command{
		Name:        "status",
		Description: "Check daemon status",
		Run:         c.daemonStatus,
	}

	daemonCmd.Subcommands["logs"] = &Command{
		Name:        "logs",
		Description: "View daemon logs",
		Run:         c.daemonLogs,
	}

	daemonCmd.Subcommands["_run"] = &Command{
		Name:        "_run",
		Description: "Internal: run daemon in foreground (used by daemon start)",
		Run:         c.runDaemon,
	}

	c.rootCmd.Subcommands["daemon"] = daemonCmd

	// Repository commands
	c.rootCmd.Subcommands["init"] = &Command{
		Name:        "init",
		Description: "Initialize a repository",
		Usage:       "multiclaude init <github-url> [path] [name]",
		Run:         c.initRepo,
	}

	c.rootCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List tracked repositories",
		Run:         c.listRepos,
	}

	// Worker commands
	workCmd := &Command{
		Name:        "work",
		Description: "Manage worker agents",
		Subcommands: make(map[string]*Command),
	}

	workCmd.Run = c.createWorker // Default action for 'work' command

	workCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List workers",
		Run:         c.listWorkers,
	}

	workCmd.Subcommands["rm"] = &Command{
		Name:        "rm",
		Description: "Remove a worker",
		Usage:       "multiclaude work rm <worker-name>",
		Run:         c.removeWorker,
	}

	c.rootCmd.Subcommands["work"] = workCmd

	// Agent commands (run from within Claude)
	agentCmd := &Command{
		Name:        "agent",
		Description: "Agent communication commands",
		Subcommands: make(map[string]*Command),
	}

	agentCmd.Subcommands["send-message"] = &Command{
		Name:        "send-message",
		Description: "Send a message to another agent",
		Run:         c.sendMessage,
	}

	agentCmd.Subcommands["list-messages"] = &Command{
		Name:        "list-messages",
		Description: "List messages",
		Run:         c.listMessages,
	}

	agentCmd.Subcommands["read-message"] = &Command{
		Name:        "read-message",
		Description: "Read a specific message",
		Run:         c.readMessage,
	}

	agentCmd.Subcommands["ack-message"] = &Command{
		Name:        "ack-message",
		Description: "Acknowledge a message",
		Run:         c.ackMessage,
	}

	agentCmd.Subcommands["complete"] = &Command{
		Name:        "complete",
		Description: "Signal worker completion",
		Run:         c.completeWorker,
	}

	c.rootCmd.Subcommands["agent"] = agentCmd

	// Attach command
	c.rootCmd.Subcommands["attach"] = &Command{
		Name:        "attach",
		Description: "Attach to an agent",
		Usage:       "multiclaude attach <agent-name> [--read-only]",
		Run:         c.attachAgent,
	}

	// Maintenance commands
	c.rootCmd.Subcommands["cleanup"] = &Command{
		Name:        "cleanup",
		Description: "Clean up orphaned resources",
		Run:         c.cleanup,
	}

	c.rootCmd.Subcommands["repair"] = &Command{
		Name:        "repair",
		Description: "Repair state after crash",
		Run:         c.repair,
	}
}

// Daemon command implementations

func (c *CLI) startDaemon(args []string) error {
	return daemon.RunDetached()
}

func (c *CLI) runDaemon(args []string) error {
	return daemon.Run()
}

func (c *CLI) stopDaemon(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "stop",
	})
	if err != nil {
		return fmt.Errorf("failed to send stop command: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("daemon stop failed: %s", resp.Error)
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

func (c *CLI) daemonStatus(args []string) error {
	// Check PID file first
	pidFile := daemon.NewPIDFile(c.paths.DaemonPID)
	running, pid, err := pidFile.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		fmt.Println("Daemon is not running")
		return nil
	}

	// Try to connect to daemon
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "status",
	})
	if err != nil {
		fmt.Printf("Daemon PID file exists (PID: %d) but daemon is not responding\n", pid)
		return nil
	}

	if !resp.Success {
		return fmt.Errorf("status check failed: %s", resp.Error)
	}

	// Pretty print status
	fmt.Println("Daemon Status:")
	if statusMap, ok := resp.Data.(map[string]interface{}); ok {
		fmt.Printf("  Running: %v\n", statusMap["running"])
		fmt.Printf("  PID: %v\n", statusMap["pid"])
		fmt.Printf("  Repos: %v\n", statusMap["repos"])
		fmt.Printf("  Agents: %v\n", statusMap["agents"])
		fmt.Printf("  Socket: %v\n", statusMap["socket_path"])
	} else {
		// Fallback: print as JSON
		jsonData, _ := json.MarshalIndent(resp.Data, "  ", "  ")
		fmt.Println(string(jsonData))
	}

	return nil
}

func (c *CLI) daemonLogs(args []string) error {
	flags, _ := ParseFlags(args)

	// Check if we should follow logs
	follow := flags["follow"] == "true" || flags["f"] == "true"

	if follow {
		// Use tail -f to follow logs
		cmd := exec.Command("tail", "-f", c.paths.DaemonLog)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Show last 50 lines
	lines := "50"
	if n, ok := flags["n"]; ok {
		lines = n
	}

	cmd := exec.Command("tail", "-n", lines, c.paths.DaemonLog)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLI) initRepo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: multiclaude init <github-url> [name]")
	}

	githubURL := args[0]

	// Parse repository name from URL if not provided
	var repoName string
	if len(args) >= 2 {
		repoName = args[1]
	} else {
		// Extract repo name from URL (e.g., github.com/user/repo -> repo)
		parts := strings.Split(githubURL, "/")
		repoName = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	fmt.Printf("Initializing repository: %s\n", repoName)
	fmt.Printf("GitHub URL: %s\n", githubURL)

	// Check if daemon is running
	client := socket.NewClient(c.paths.DaemonSock)
	_, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		return fmt.Errorf("daemon not running. Start it with: multiclaude start")
	}

	// Clone repository
	repoPath := c.paths.RepoDir(repoName)
	fmt.Printf("Cloning to: %s\n", repoPath)

	cmd := exec.Command("git", "clone", githubURL, repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Create tmux session
	tmuxSession := fmt.Sprintf("mc-%s", repoName)

	fmt.Printf("Creating tmux session: %s\n", tmuxSession)

	// Create session with supervisor window
	cmd = exec.Command("tmux", "new-session", "-d", "-s", tmuxSession, "-n", "supervisor", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Create merge-queue window
	cmd = exec.Command("tmux", "new-window", "-t", tmuxSession, "-n", "merge-queue", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create merge-queue window: %w", err)
	}

	// Add repository to daemon state
	resp, err := client.Send(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         repoName,
			"github_url":   githubURL,
			"tmux_session": tmuxSession,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register repository with daemon: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register repository: %s", resp.Error)
	}

	// Add supervisor agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "supervisor",
			"type":          "supervisor",
			"worktree_path": repoPath,
			"tmux_window":   "supervisor",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register supervisor: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register supervisor: %s", resp.Error)
	}

	// Add merge-queue agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "merge-queue",
			"type":          "merge-queue",
			"worktree_path": repoPath,
			"tmux_window":   "merge-queue",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register merge-queue: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register merge-queue: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Repository initialized successfully!")
	fmt.Printf("  Tmux session: %s\n", tmuxSession)
	fmt.Printf("  Agents: supervisor, merge-queue\n")
	fmt.Printf("\nAttach to session: tmux attach -t %s\n", tmuxSession)
	fmt.Printf("Or use: multiclaude attach supervisor\n")

	return nil
}

func (c *CLI) listRepos(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "list_repos"})
	if err != nil {
		return fmt.Errorf("failed to list repos: %w (is daemon running?)", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to list repos: %s", resp.Error)
	}

	repos, ok := resp.Data.([]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	if len(repos) == 0 {
		fmt.Println("No repositories tracked")
		fmt.Println("\nInitialize a repository with: multiclaude init <github-url>")
		return nil
	}

	fmt.Printf("Tracked repositories (%d):\n", len(repos))
	for _, repo := range repos {
		if repoStr, ok := repo.(string); ok {
			fmt.Printf("  - %s\n", repoStr)
		}
	}

	return nil
}

func (c *CLI) createWorker(args []string) error {
	flags, posArgs := ParseFlags(args)

	// Get task description
	task := strings.Join(posArgs, " ")
	if task == "" {
		return fmt.Errorf("usage: multiclaude work <task description>")
	}

	// Determine repository (from flag or current directory)
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Check if we're in a tracked repo
		repos := c.getReposList()
		for _, repo := range repos {
			repoPath := c.paths.RepoDir(repo)
			if strings.HasPrefix(cwd, repoPath) {
				repoName = repo
				break
			}
		}

		if repoName == "" {
			return fmt.Errorf("not in a tracked repository. Use --repo flag or run from repo directory")
		}
	}

	// Generate worker name
	workerName := fmt.Sprintf("worker-%d", os.Getpid()%10000)
	if name, ok := flags["name"]; ok {
		workerName = name
	}

	fmt.Printf("Creating worker '%s' in repo '%s'\n", workerName, repoName)
	fmt.Printf("Task: %s\n", task)

	// Get repository path
	repoPath := c.paths.RepoDir(repoName)

	// Create worktree
	wt := worktree.NewManager(repoPath)
	wtPath := c.paths.AgentWorktree(repoName, workerName)
	branchName := fmt.Sprintf("work/%s", workerName)

	fmt.Printf("Creating worktree at: %s\n", wtPath)
	if err := wt.CreateNewBranch(wtPath, branchName, "HEAD"); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Get repository info to determine tmux session
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get repo info: %w (is daemon running?)", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to get repo info: %s", resp.Error)
	}

	// Get tmux session name (it's mc-<reponame>)
	tmuxSession := fmt.Sprintf("mc-%s", repoName)

	// Create tmux window for worker
	fmt.Printf("Creating tmux window: %s\n", workerName)
	cmd := exec.Command("tmux", "new-window", "-t", tmuxSession, "-n", workerName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Register worker with daemon
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         workerName,
			"type":          "worker",
			"worktree_path": wtPath,
			"tmux_window":   workerName,
			"task":          task,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register worker: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Worker created successfully!")
	fmt.Printf("  Name: %s\n", workerName)
	fmt.Printf("  Branch: %s\n", branchName)
	fmt.Printf("  Worktree: %s\n", wtPath)
	fmt.Printf("\nAttach to worker: tmux select-window -t %s:%s\n", tmuxSession, workerName)
	fmt.Printf("Or use: multiclaude attach %s\n", workerName)

	return nil
}

func (c *CLI) listWorkers(args []string) error {
	flags, _ := ParseFlags(args)

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// List all repos
		repos := c.getReposList()
		if len(repos) == 0 {
			fmt.Println("No repositories tracked")
			return nil
		}

		// If only one repo, use it
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return fmt.Errorf("multiple repos exist. Use --repo flag to specify which one")
		}
	}

	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list workers: %w (is daemon running?)", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to list workers: %s", resp.Error)
	}

	agents, ok := resp.Data.([]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	// Filter for workers only
	workers := []map[string]interface{}{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			if agentType, _ := agentMap["type"].(string); agentType == "worker" {
				workers = append(workers, agentMap)
			}
		}
	}

	if len(workers) == 0 {
		fmt.Printf("No workers in repository '%s'\n", repoName)
		fmt.Println("\nCreate a worker with: multiclaude work <task>")
		return nil
	}

	fmt.Printf("Workers in '%s' (%d):\n", repoName, len(workers))
	for _, worker := range workers {
		name := worker["name"].(string)
		task := worker["task"].(string)
		fmt.Printf("  - %s: %s\n", name, task)
	}

	return nil
}

func (c *CLI) removeWorker(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: multiclaude work rm <worker-name>")
	}

	workerName := args[0]

	// Determine repository
	flags, _ := ParseFlags(args[1:])
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from tracked repos
		repos := c.getReposList()
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return fmt.Errorf("multiple repos exist. Use --repo flag to specify which one")
		}
	}

	fmt.Printf("Removing worker '%s' from repo '%s'\n", workerName, repoName)

	// Get worker info
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get worker info: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to get worker info: %s", resp.Error)
	}

	// Find worker
	agents, _ := resp.Data.([]interface{})
	var workerInfo map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			if name, _ := agentMap["name"].(string); name == workerName {
				workerInfo = agentMap
				break
			}
		}
	}

	if workerInfo == nil {
		return fmt.Errorf("worker '%s' not found", workerName)
	}

	// Kill tmux window
	tmuxSession := fmt.Sprintf("mc-%s", repoName)
	tmuxWindow := workerInfo["tmux_window"].(string)
	fmt.Printf("Killing tmux window: %s\n", tmuxWindow)
	cmd := exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to kill tmux window: %v\n", err)
	}

	// Remove worktree
	wtPath := workerInfo["worktree_path"].(string)
	repoPath := c.paths.RepoDir(repoName)
	wt := worktree.NewManager(repoPath)

	fmt.Printf("Removing worktree: %s\n", wtPath)
	if err := wt.Remove(wtPath, false); err != nil {
		fmt.Printf("Warning: failed to remove worktree: %v\n", err)
	}

	// Unregister from daemon
	resp, err = client.Send(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"repo":  repoName,
			"agent": workerName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to unregister worker: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to unregister worker: %s", resp.Error)
	}

	fmt.Println("✓ Worker removed successfully")
	return nil
}

// getReposList is a helper to get the list of repos
func (c *CLI) getReposList() []string {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "list_repos"})
	if err != nil {
		return []string{}
	}

	if !resp.Success {
		return []string{}
	}

	repos, ok := resp.Data.([]interface{})
	if !ok {
		return []string{}
	}

	result := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repoStr, ok := repo.(string); ok {
			result = append(result, repoStr)
		}
	}

	return result
}

func (c *CLI) sendMessage(args []string) error {
	fmt.Println("Sending message... (not yet implemented)")
	return nil
}

func (c *CLI) listMessages(args []string) error {
	fmt.Println("Listing messages... (not yet implemented)")
	return nil
}

func (c *CLI) readMessage(args []string) error {
	fmt.Println("Reading message... (not yet implemented)")
	return nil
}

func (c *CLI) ackMessage(args []string) error {
	fmt.Println("Acknowledging message... (not yet implemented)")
	return nil
}

func (c *CLI) completeWorker(args []string) error {
	fmt.Println("Completing worker... (not yet implemented)")
	return nil
}

func (c *CLI) attachAgent(args []string) error {
	fmt.Println("Attaching to agent... (not yet implemented)")
	return nil
}

func (c *CLI) cleanup(args []string) error {
	fmt.Println("Cleaning up... (not yet implemented)")
	return nil
}

func (c *CLI) repair(args []string) error {
	fmt.Println("Repairing state... (not yet implemented)")
	return nil
}

// ParseFlags is a simple flag parser
func ParseFlags(args []string) (map[string]string, []string) {
	flags := make(map[string]string)
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			// Long flag
			flag := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags[flag] = args[i+1]
				i++
			} else {
				flags[flag] = "true"
			}
		} else if strings.HasPrefix(arg, "-") {
			// Short flag
			flag := strings.TrimPrefix(arg, "-")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags[flag] = args[i+1]
				i++
			} else {
				flags[flag] = "true"
			}
		} else {
			positional = append(positional, arg)
		}
	}

	return flags, positional
}
