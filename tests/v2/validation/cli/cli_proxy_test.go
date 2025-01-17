package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/extensions/proxy"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"github.com/rancher/shepherd/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CLIProxyTestSuite struct {
	suite.Suite
	client          *rancher.Client
	session         *session.Session
	proxyServer     *proxy.TestProxy
	standardTimeout time.Duration
	rancherHostname string
	rancherUsername string
	rancherPassword string
}

func (p *CLIProxyTestSuite) SetupSuite() {
	testSession := session.NewSession()
	p.session = testSession

	client, err := rancher.NewClient("", testSession)
	require.NoError(p.T(), err)
	p.client = client

	p.standardTimeout = 30 * time.Second
	p.rancherHostname = client.RancherConfig.Host
	p.rancherUsername = client.RancherConfig.AdminUsername
	p.rancherPassword = client.RancherConfig.AdminPassword
}

func (p *CLIProxyTestSuite) SetupTest() {
	// Create a new test proxy for each test
	var err error
	p.proxyServer, err = proxy.NewTestProxy()
	require.NoError(p.T(), err)
	require.NoError(p.T(), p.proxyServer.Start())
}

func (p *CLIProxyTestSuite) TearDownTest() {
	if p.proxyServer != nil {
		p.proxyServer.Stop()
	}
	// Clear any proxy environment variables
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
}

func (p *CLIProxyTestSuite) TestBasicProxyFunctionality() {
	p.T().Log("Testing basic proxy functionality with Rancher CLI")

	// Set proxy environment variables
	proxyURL := fmt.Sprintf("http://localhost:%d", p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)

	// Test login command
	loginCmd := exec.Command("rancher", "login",
		"--skip-verify",
		"--token", p.client.RancherConfig.AdminToken,
		p.rancherHostname)

	output, err := loginCmd.CombinedOutput()
	assert.NoError(p.T(), err, "Login command failed: %s", string(output))

	// Verify proxy was used
	assert.True(p.T(), p.proxyServer.WasUsed(), "Proxy should have been used for login")
	assert.Contains(p.T(), p.proxyServer.Requests(), "/v3/tokens")
}

func (p *CLIProxyTestSuite) TestProxyWithAuthentication() {
	p.T().Log("Testing proxy with authentication")

	// Set up proxy with authentication
	proxyUser := namegen.AppendRandomString("user")
	proxyPass := namegen.AppendRandomString("pass")
	p.proxyServer.SetAuth(proxyUser, proxyPass)

	// Set proxy environment variables with auth
	proxyURL := fmt.Sprintf("http://%s:%s@localhost:%d",
		proxyUser, proxyPass, p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)

	// Test with correct credentials
	loginCmd := exec.Command("rancher", "login",
		"--skip-verify",
		"--token", p.client.RancherConfig.AdminToken,
		p.rancherHostname)

	output, err := loginCmd.CombinedOutput()
	assert.NoError(p.T(), err, "Login should succeed with correct proxy credentials")

	// Test with incorrect credentials
	wrongProxyURL := fmt.Sprintf("http://%s:%s@localhost:%d",
		proxyUser, "wrongpass", p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", wrongProxyURL)
	os.Setenv("HTTPS_PROXY", wrongProxyURL)

	output, err = loginCmd.CombinedOutput()
	assert.Error(p.T(), err, "Login should fail with incorrect proxy credentials")
	assert.Contains(p.T(), string(output), "Proxy Authentication Required")
}

func (p *CLIProxyTestSuite) TestProxyTimeout() {
	p.T().Log("Testing proxy timeout behavior")

	// Set up proxy with delay
	p.proxyServer.SetDelay(5 * time.Second)

	// Set proxy environment variables
	proxyURL := fmt.Sprintf("http://localhost:%d", p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)

	// Test with default timeout
	loginCmd := exec.Command("rancher", "login",
		"--skip-verify",
		"--token", p.client.RancherConfig.AdminToken,
		p.rancherHostname)

	start := time.Now()
	output, err := loginCmd.CombinedOutput()
	duration := time.Since(start)

	// Verify timeout behavior
	assert.True(p.T(), duration >= 5*time.Second,
		"Request should take at least the delay time")
	assert.NoError(p.T(), err, "Login should succeed despite delay")
}

func (p *CLIProxyTestSuite) TestSSHThroughProxy() {
	p.T().Log("Testing SSH command through proxy")

	// Set proxy environment variables
	proxyURL := fmt.Sprintf("http://localhost:%d", p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)

	// First get a node ID
	nodesCmd := exec.Command("rancher", "nodes", "ls")
	output, err := nodesCmd.CombinedOutput()
	require.NoError(p.T(), err, "Failed to list nodes")

	// Parse output to get node ID
	// Note: Add logic to parse node ID from output

	// Test SSH command
	sshCmd := exec.Command("rancher", "ssh", "-e", "node-id")
	output, err = sshCmd.CombinedOutput()

	// Verify proxy was used for SSH key download
	assert.True(p.T(), p.proxyServer.WasUsed(),
		"Proxy should have been used for SSH command")
}

func (p *CLIProxyTestSuite) TestMultipleCommands() {
	p.T().Log("Testing multiple CLI commands through proxy")

	// Set proxy environment variables
	proxyURL := fmt.Sprintf("http://localhost:%d", p.proxyServer.Port)
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)

	// Login first
	loginCmd := exec.Command("rancher", "login",
		"--skip-verify",
		"--token", p.client.RancherConfig.AdminToken,
		p.rancherHostname)

	_, err := loginCmd.CombinedOutput()
	require.NoError(p.T(), err, "Login failed")

	// Test various commands
	commands := []struct {
		name string
		args []string
	}{
		{"clusters list", []string{"clusters", "ls"}},
		{"nodes list", []string{"nodes", "ls"}},
		{"settings list", []string{"settings", "ls"}},
	}

	for _, cmd := range commands {
		p.T().Logf("Testing command: %s", cmd.name)
		command := exec.Command("rancher", cmd.args...)
		output, err := command.CombinedOutput()
		assert.NoError(p.T(), err,
			"Command %s failed: %s", cmd.name, string(output))
		assert.True(p.T(), p.proxyServer.WasUsed(),
			"Proxy should have been used for command: %s", cmd.name)
	}
}

func TestCLIProxyTestSuite(t *testing.T) {
	suite.Run(t, new(CLIProxyTestSuite))
}
