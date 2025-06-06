package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DependencyManager handles system dependencies and their installation
type DependencyManager struct {
	installationLog []string
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager() *DependencyManager {
	return &DependencyManager{
		installationLog: make([]string, 0),
	}
}

// CheckAndInstallDependencies checks and installs all required dependencies
func (dm *DependencyManager) CheckAndInstallDependencies() error {
	dm.log("Starting dependency check...")

	// Check Docker first (most critical)
	if err := dm.ensureDocker(); err != nil {
		return fmt.Errorf("docker setup failed: %w", err)
	}

	// Check other dependencies
	dependencies := []struct {
		name     string
		checker  func() bool
		installer func() error
	}{
		{"curl", dm.IsCurlInstalled, dm.installCurl},
		{"git", dm.IsGitInstalled, dm.installGit},
	}

	for _, dep := range dependencies {
		if !dep.checker() {
			dm.log(fmt.Sprintf("%s not found, installing...", dep.name))
			if err := dep.installer(); err != nil {
				dm.log(fmt.Sprintf("Failed to install %s: %v", dep.name, err))
				// Continue with other dependencies
			} else {
				dm.log(fmt.Sprintf("%s installed successfully", dep.name))
			}
		} else {
			dm.log(fmt.Sprintf("%s is already installed", dep.name))
		}
	}

	return nil
}

// ensureDocker ensures Docker is installed and running
func (dm *DependencyManager) ensureDocker() error {
	if !dm.IsDockerInstalled() {
		dm.log("Docker not found, installing...")
		if err := dm.installDocker(); err != nil {
			return fmt.Errorf("failed to install Docker: %w", err)
		}
	} else {
		dm.log("Docker is installed")
	}

	// Check if Docker daemon is running
	if !dm.IsDockerRunning() {
		dm.log("Docker daemon not running, attempting to start...")
		if err := dm.StartDockerDaemon(); err != nil {
			return fmt.Errorf("failed to start Docker daemon: %w", err)
		}
	} else {
		dm.log("Docker daemon is running")
	}

	return nil
}

// IsDockerInstalled checks if Docker is installed
func (dm *DependencyManager) IsDockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// IsDockerRunning checks if Docker daemon is running
func (dm *DependencyManager) IsDockerRunning() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// installDocker installs Docker based on the operating system
func (dm *DependencyManager) installDocker() error {
	switch runtime.GOOS {
	case "linux":
		return dm.installDockerLinux()
	case "darwin":
		return dm.installDockerMacOS()
	case "windows":
		return dm.installDockerWindows()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// installDockerLinux installs Docker on Linux
func (dm *DependencyManager) installDockerLinux() error {
	distro := dm.getLinuxDistribution()
	dm.log(fmt.Sprintf("Detected Linux distribution: %s", distro))

	switch {
	case strings.Contains(distro, "ubuntu") || strings.Contains(distro, "debian"):
		return dm.installDockerUbuntuDebian()
	case strings.Contains(distro, "centos") || strings.Contains(distro, "rhel") || strings.Contains(distro, "fedora"):
		return dm.installDockerCentOSRHEL()
	case strings.Contains(distro, "arch"):
		return dm.installDockerArch()
	default:
		// Try the universal script
		dm.log("Unknown distribution, trying universal installation script...")
		return dm.installDockerUniversal()
	}
}

// installDockerUbuntuDebian installs Docker on Ubuntu/Debian
func (dm *DependencyManager) installDockerUbuntuDebian() error {
	commands := [][]string{
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "install", "-y", "ca-certificates", "curl", "gnupg"},
		{"sudo", "install", "-m", "0755", "-d", "/etc/apt/keyrings"},
		{"curl", "-fsSL", "https://download.docker.com/linux/ubuntu/gpg", "|", "sudo", "gpg", "--dearmor", "-o", "/etc/apt/keyrings/docker.gpg"},
		{"sudo", "chmod", "a+r", "/etc/apt/keyrings/docker.gpg"},
	}

	for _, cmd := range commands {
		if err := dm.runCommand(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}

	// Add Docker repository
	cmd := `echo "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null`
	if err := dm.runShellCommand(cmd); err != nil {
		return err
	}

	// Install Docker
	finalCommands := [][]string{
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"},
		{"sudo", "usermod", "-aG", "docker", os.Getenv("USER")},
	}

	for _, cmd := range finalCommands {
		if err := dm.runCommand(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}

	return nil
}

// installDockerCentOSRHEL installs Docker on CentOS/RHEL/Fedora
func (dm *DependencyManager) installDockerCentOSRHEL() error {
	commands := [][]string{
		{"sudo", "yum", "install", "-y", "yum-utils"},
		{"sudo", "yum-config-manager", "--add-repo", "https://download.docker.com/linux/centos/docker-ce.repo"},
		{"sudo", "yum", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"},
		{"sudo", "usermod", "-aG", "docker", os.Getenv("USER")},
	}

	for _, cmd := range commands {
		if err := dm.runCommand(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}

	return nil
}

// installDockerArch installs Docker on Arch Linux
func (dm *DependencyManager) installDockerArch() error {
	commands := [][]string{
		{"sudo", "pacman", "-S", "--noconfirm", "docker", "docker-compose"},
		{"sudo", "usermod", "-aG", "docker", os.Getenv("USER")},
	}

	for _, cmd := range commands {
		if err := dm.runCommand(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}

	return nil
}

// installDockerUniversal uses Docker's universal installation script
func (dm *DependencyManager) installDockerUniversal() error {
	cmd := "curl -fsSL https://get.docker.com | sh"
	if err := dm.runShellCommand(cmd); err != nil {
		return err
	}

	// Add user to docker group
	return dm.runCommand("sudo", "usermod", "-aG", "docker", os.Getenv("USER"))
}

// installDockerMacOS installs Docker on macOS
func (dm *DependencyManager) installDockerMacOS() error {
	// Check if Homebrew is available
	if _, err := exec.LookPath("brew"); err == nil {
		dm.log("Installing Docker via Homebrew...")
		return dm.runCommand("brew", "install", "--cask", "docker")
	}

	// Provide manual installation instructions
	return fmt.Errorf("please install Docker Desktop for Mac from https://docs.docker.com/desktop/install/mac-install/")
}

// installDockerWindows provides instructions for Windows
func (dm *DependencyManager) installDockerWindows() error {
	return fmt.Errorf("please install Docker Desktop for Windows from https://docs.docker.com/desktop/install/windows-install/")
}

// StartDockerDaemon starts the Docker daemon
func (dm *DependencyManager) StartDockerDaemon() error {
	switch runtime.GOOS {
	case "linux":
		// Try systemctl first, then service command
		if err := dm.runCommand("sudo", "systemctl", "start", "docker"); err != nil {
			dm.log("systemctl failed, trying service command...")
			return dm.runCommand("sudo", "service", "docker", "start")
		}
		
		// Enable Docker to start on boot
		dm.runCommand("sudo", "systemctl", "enable", "docker")
		
		// Wait for Docker to be ready
		return dm.waitForDockerReady()
	case "darwin":
		return fmt.Errorf("please start Docker Desktop manually on macOS")
	case "windows":
		return fmt.Errorf("please start Docker Desktop manually on Windows")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// waitForDockerReady waits for Docker daemon to be ready
func (dm *DependencyManager) waitForDockerReady() error {
	dm.log("Waiting for Docker daemon to be ready...")
	
	for i := 0; i < 30; i++ {
		if dm.IsDockerRunning() {
			dm.log("Docker daemon is ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	
	return fmt.Errorf("timeout waiting for Docker daemon to start")
}

// getLinuxDistribution detects the Linux distribution
func (dm *DependencyManager) getLinuxDistribution() string {
	// Try /etc/os-release first
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(data)
		if strings.Contains(strings.ToLower(content), "ubuntu") {
			return "ubuntu"
		}
		if strings.Contains(strings.ToLower(content), "debian") {
			return "debian"
		}
		if strings.Contains(strings.ToLower(content), "centos") {
			return "centos"
		}
		if strings.Contains(strings.ToLower(content), "rhel") || strings.Contains(strings.ToLower(content), "red hat") {
			return "rhel"
		}
		if strings.Contains(strings.ToLower(content), "fedora") {
			return "fedora"
		}
		if strings.Contains(strings.ToLower(content), "arch") {
			return "arch"
		}
	}

	// Fallback methods
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "debian"
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return "rhel"
	}
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return "arch"
	}

	return "unknown"
}

// Other dependency checkers and installers
func (dm *DependencyManager) IsCurlInstalled() bool {
	_, err := exec.LookPath("curl")
	return err == nil
}

func (dm *DependencyManager) installCurl() error {
	switch runtime.GOOS {
	case "linux":
		distro := dm.getLinuxDistribution()
		switch {
		case strings.Contains(distro, "ubuntu") || strings.Contains(distro, "debian"):
			return dm.runCommand("sudo", "apt-get", "install", "-y", "curl")
		case strings.Contains(distro, "centos") || strings.Contains(distro, "rhel") || strings.Contains(distro, "fedora"):
			return dm.runCommand("sudo", "yum", "install", "-y", "curl")
		case strings.Contains(distro, "arch"):
			return dm.runCommand("sudo", "pacman", "-S", "--noconfirm", "curl")
		}
	case "darwin":
		return dm.runCommand("brew", "install", "curl")
	}
	return fmt.Errorf("unsupported platform for curl installation")
}

func (dm *DependencyManager) IsGitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func (dm *DependencyManager) installGit() error {
	switch runtime.GOOS {
	case "linux":
		distro := dm.getLinuxDistribution()
		switch {
		case strings.Contains(distro, "ubuntu") || strings.Contains(distro, "debian"):
			return dm.runCommand("sudo", "apt-get", "install", "-y", "git")
		case strings.Contains(distro, "centos") || strings.Contains(distro, "rhel") || strings.Contains(distro, "fedora"):
			return dm.runCommand("sudo", "yum", "install", "-y", "git")
		case strings.Contains(distro, "arch"):
			return dm.runCommand("sudo", "pacman", "-S", "--noconfirm", "git")
		}
	case "darwin":
		return dm.runCommand("brew", "install", "git")
	}
	return fmt.Errorf("unsupported platform for git installation")
}

// Utility methods
func (dm *DependencyManager) runCommand(name string, args ...string) error {
	dm.log(fmt.Sprintf("Running: %s %s", name, strings.Join(args, " ")))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (dm *DependencyManager) runShellCommand(command string) error {
	dm.log(fmt.Sprintf("Running shell command: %s", command))
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (dm *DependencyManager) log(message string) {
	timestamp := time.Now().Format("15:04:05")
	logMessage := fmt.Sprintf("[%s] %s", timestamp, message)
	dm.installationLog = append(dm.installationLog, logMessage)
	fmt.Println(logMessage)
}

// GetInstallationLog returns the installation log
func (dm *DependencyManager) GetInstallationLog() []string {
	return dm.installationLog
}

// PromptUserPermission asks user for permission to install dependencies
func (dm *DependencyManager) PromptUserPermission() bool {
	fmt.Print("TRMS needs to install system dependencies (Docker, curl, git). Continue? [y/N]: ")
	
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return response == "y" || response == "yes"
	}
	
	return false
}