package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

const CdrFileName = "clipboard-data-receiver"
const CdrFileNameForMac = "clipboard-data-receiver"
const cdrFileNameForWindows = "clipboard-data-receiver.exe"

// Download URL for clipboard-data-receiver
const downloadURLCdrPattern = "https://github.com/mikoto2000/clipboard-data-receiver/releases/download/{{ .TagName }}/clipboard-data-receiver.linux-amd64"
const downloadURLCdrPatternForMac = "https://github.com/mikoto2000/clipboard-data-receiver/releases/download/{{ .TagName }}/clipboard-data-receiver.darwin-amd64"
const downloadURLCdrPatternForWindows = "https://github.com/mikoto2000/clipboard-data-receiver/releases/download/{{ .TagName }}/clipboard-data-receiver.windows-amd64.exe"

const vimScriptTemplateSendToCdr = `if !has("nvim")
  function! SendToCdr(register) abort
    let text = getreg(a:register)
    let l:channelToCdr = ch_open("host.docker.internal:{{ .Port }}", {"mode": "raw"})
    call ch_sendraw(channelToCdr, l:text, {})
    call ch_close(l:channelToCdr)
  endfunction
endif`

const vimNoCdrScriptTemplateSendToCdr = `if !has("nvim")
  function! SendToCdr(register) abort
    " nop
  endfunction
endif`

const luaScriptTemplateSendToCdr = `function SendToCdr(register)
  local text = vim.fn.getreg(register)
  local uv = vim.loop
  local host = "host.docker.internal"
  local port = {{ .Port }}

  -- Resolve hostname
  uv.getaddrinfo(host, nil, { socktype = "STREAM" }, function(err, res)
    if err then
      print("DNS resolution error: " .. err)
      return
    end

    local addr = res[1].addr -- Resolved IP address
    local client = uv.new_tcp()

    -- TCP connection
    client:connect(addr, port, function(connect_err)
      if connect_err then
        print("Connection error: " .. connect_err)
        return
      end

      -- Send data
      client:write(text, function(write_err)
        if write_err then
          print("Write error: " .. write_err)
        end

        -- Close connection
        client:shutdown(function(shutdown_err)
          if shutdown_err then
            print("Shutdown error: " .. shutdown_err)
          end
          client:close()
        end)
      end)
    end)
  end)
end
`
const luaNoCdrScriptTemplateSendToCdr = `function SendToCdr(register)
  -- nop
end
`

// Tool information for clipboard-data-receiver
var CDR = func(services InstallerUseServices) Tool {

	// Determine if running on WSL,
	// and download .exe if running on WSL
	var cdrFileName string
	var tmpl *template.Template
	var err error
	if util.IsWsl() {
		cdrFileName = cdrFileNameForWindows
		tmpl, err = template.New("ducp").Parse(downloadURLCdrPatternForWindows)
	} else if runtime.GOOS == "darwin" {
		cdrFileName = CdrFileNameForMac
		tmpl, err = template.New("ducp").Parse(downloadURLCdrPatternForMac)
	} else {
		cdrFileName = CdrFileName
		tmpl, err = template.New("ducp").Parse(downloadURLCdrPattern)
	}
	if err != nil {
		return Tool{
			FileName: cdrFileName,
			CalculateDownloadURL: func(_ string) (string, error) {
				return "", err
			},
			installFunc: func(downloadFunc func(downloadURL string, destPath string) error, downloadURL string, filePath string, containerArch string) (string, error) {
				return "", err
			},
			DownloadFunc: services.Download,
		}
	}

	// Return the cdr structure actually used
	return Tool{
		FileName: cdrFileName,
		CalculateDownloadURL: func(_ string) (string, error) {
			latestTagName, err := services.GetLatestReleaseFromGitHub("mikoto2000", "clipboard-data-receiver")
			if err != nil {
				return "", err
			}

			tmplParams := map[string]string{"TagName": latestTagName}
			var downloadURL strings.Builder
			err = tmpl.Execute(&downloadURL, tmplParams)
			if err != nil {
				return "", err
			}
			return downloadURL.String(), nil
		},
		installFunc: func(downloadFunc func(downloadURL string, destPath string) error, downloadURL string, filePath string, containerArch string) (string, error) {
			return simpleInstall(downloadFunc, downloadURL, filePath)
		},
		DownloadFunc: services.Download,
	}
}

// Start clipboard-data-receiver.
// Save pid and port files to configFileDir.
func RunCdr(cdrPath string, configFileDir string) (int, int, error) {
	// Construct pid and port file paths from configFileDir
	pidFile := filepath.Join(configFileDir, "pid")
	portFile := filepath.Join(configFileDir, "port")

	// Windows check
	if runtime.GOOS == "windows" {
		return runCdrForNative(cdrPath, pidFile, portFile)
	} else {
		if util.IsWsl() {
			return runCdrForWsl(cdrPath, pidFile, portFile)
		} else {
			return runCdrForNative(cdrPath, pidFile, portFile)
		}
	}
}

// Process for running clipboard-data-receiver in a non-WSL environment
func runCdrForNative(cdrPath string, pidFile string, portFile string) (int, int, error) {
	fmt.Println("\""+cdrPath+"\"", "--pid-file", pidFile, "--port-file", portFile, "--random-port")
	cdrRunCommand := exec.Command(cdrPath, "--pid-file", pidFile, "--port-file", portFile, "--random-port")
	var stdout strings.Builder
	cdrRunCommand.Stdout = &stdout
	err := cdrRunCommand.Start()
	if err != nil {
		return 0, 0, err
	}

	// Wait for clipboard-data-receiver output
	// 10 second timeout
	var pid, port int
	for i := 0; i < 10; i++ {
		pid, _, port, err = GetProcessInfo(stdout.String())
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		} else {
			break
		}
	}

	return pid, port, nil
}

func runCdrForWsl(cdrPath string, pidFile string, portFile string) (int, int, error) {
	// Execute clipboard-data-receiver.exe
	commandString := fmt.Sprintf("%s --random-port --pid-file $(wslpath -w %s) --port-file $(wslpath -w %s)", cdrPath, pidFile, portFile)
	fmt.Println(commandString)
	cdrRunCommand := exec.Command("sh", "-c", commandString)
	var stdout strings.Builder
	cdrRunCommand.Stdout = &stdout
	err := cdrRunCommand.Start()
	if err != nil {
		return 0, 0, err
	}

	// Wait for clipboard-data-receiver output

	// Wait for PID file output
	// Currently, PID and port files cannot be cleaned up on Windows,
	// so we wait for 1 second assuming they will be generated.
	var pid int
	for i := 0; i < 10; i++ {
		pidFileContentBytes, err := os.ReadFile(pidFile)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		pid, err = strconv.Atoi(string(pidFileContentBytes))
		if err != nil {
			return 0, 0, err
		}

		break
	}

	// Wait for port file output
	var port int
	for i := 0; i < 10; i++ {
		portFileContentBytes, err := os.ReadFile(portFile)
		if err != nil {
			// Wait for port file output
			time.Sleep(1 * time.Second)
			continue
		}

		port, err = strconv.Atoi(string(portFileContentBytes))
		if err != nil {
			return 0, 0, err
		}

		break
	}

	return pid, port, nil
}

func KillCdr(pid int) error {
	if util.IsWsl() {
		commandString := fmt.Sprintf("Stop-Process -Id %d -Force", pid)
		fmt.Printf("Stop clipboard-data-receiver: %s\n", commandString)
		cdrRunCommand := exec.Command("powershell.exe", "-Command", commandString)
		err := cdrRunCommand.Start()
		if err != nil {
			return err
		}
	} else {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		err = process.Kill()
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateSendToTCP(configDir string, port int, noCdr bool, nvim bool) (string, error) {
	// Construct SendToTcp.vim string
	var tmpl *template.Template
	var err error
	if !noCdr {
		// cdr enabled
		if nvim {
			tmpl, err = template.New("SendToTcp").Parse(luaScriptTemplateSendToCdr)
			if err != nil {
				return "", err
			}
		} else {
			tmpl, err = template.New("SendToTcp").Parse(vimScriptTemplateSendToCdr)
			if err != nil {
				return "", err
			}
		}
	} else {
		// cdr disabled
		if nvim {
			tmpl, err = template.New("SendToTcp").Parse(luaNoCdrScriptTemplateSendToCdr)
			if err != nil {
				return "", err
			}
		} else {
			tmpl, err = template.New("SendToTcp").Parse(vimNoCdrScriptTemplateSendToCdr)
			if err != nil {
				return "", err
			}
		}
	}

	tmplParams := map[string]int{"Port": port}
	var sendToTCPString strings.Builder
	err = tmpl.Execute(&sendToTCPString, tmplParams)
	if err != nil {
		return "", err
	}

	// Output to file
	var sendToTCP string
	if nvim {
		sendToTCP = filepath.Join(configDir, "SendToTcp.lua")
	} else {
		sendToTCP = filepath.Join(configDir, "SendToTcp.vim")
	}
	err = os.WriteFile(sendToTCP, []byte(sendToTCPString.String()), 0666)
	if err != nil {
		return "", err
	}

	// Return the created file
	return sendToTCP, nil
}
