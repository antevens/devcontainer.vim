package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anmitsu/go-shlex"
	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"

	"github.com/mikoto2000/devcontainer.vim/v3/devcontainer"
	"github.com/mikoto2000/devcontainer.vim/v3/oras"
	"github.com/mikoto2000/devcontainer.vim/v3/tools"
	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

type IndexRoot struct {
	Collections []Collection `json:"collections"`
}

type Collection struct {
	Templates []AvailableTemplateItem `json:"templates"`
}

type AvailableTemplateItem struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

const containerCommand = "docker"

var version = "dev"

const envDevcontainerVimType = "DEVCONTAINER_VIM_TYPE"
const envDevcontainerShellType = "DEVCONTAINER_SHELL_TYPE"

const flagNameLicense = "license"
const flagNameNeoVim = "nvim"
const flagNameNoCdr = "nocdr"
const flagNameNoPf = "nopf"
const flagNameNoTmux = "notmux"
const flagNameShell = "shell"
const flagNameArch = "arch"

const flagNameGenerate = "generate"
const flagNameHome = "home"
const flagNameOutput = "output"
const flagNameOpen = "open"

//go:embed LICENSE
var license string

//go:embed NOTICE
var notice string

//go:embed bash_complete_func.bash
var bash_complete_func string

//go:embed devcontainer.vim.template.json
var devcontainerVimJSONTemplate string

const runargsContent = "-v \"$(pwd):/work\" -v \"${HOME}/.gitconfig:/root/.gitconfig\" -v \"${HOME}/.ssh:/root/.ssh\" --workdir /work"

//go:embed vimrc.template.vim
var additionalVimrc string

const appName = "devcontainer.vim"

func main() {
	// Update environment variables so that HOME directory can be specified with ${ localEnv:HOME } on Windows as well.
	if runtime.GOOS == "windows" {
		fmt.Printf("Set environment variable HOME to %s.\n", os.Getenv("USERPROFILE"))
		os.Setenv("HOME", os.Getenv("USERPROFILE"))
	}

	// Parse command line options

	// Create directories for devcontainer.vim
	// 1. Directory for user configuration
	//    `os.UserConfigDir` + `devcontainer.vim`
	// 2. Directory for user cache
	//    `os.UserCacheDir` + `devcontainer.vim`
	appConfigDir, err := util.CreateConfigDirectory(os.UserConfigDir, appName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
		os.Exit(1)
	}
	appCacheDir, binDir, configDirForDocker, configDirForDevcontainer, err := util.CreateCacheDirectory(os.UserCacheDir, appName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cache directory: %v\n", err)
		os.Exit(1)
	}

	// Construct the output destination for the vimrc file
	// Output vimrc (do nothing if it already exists)
	vimrc := filepath.Join(appConfigDir, "vimrc")
	if !util.IsExists(vimrc) {
		err := util.CreateFileWithContents(vimrc, additionalVimrc, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating vimrc file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated additional vimrc to: %s\n", vimrc)
	}

	// Construct the output destination for the runargs file
	// Output runargs (do nothing if it already exists)
	runargs := filepath.Join(appConfigDir, "runargs")
	if !util.IsExists(runargs) {
		err := util.CreateFileWithContents(runargs, runargsContent, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating runargs file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated additional runargs to: %s\n", runargs)
	}

	devcontainerVimArgProcess := (&cli.App{
		Name:                   "devcontainer.vim",
		Usage:                  "devcontainer for vim.",
		Version:                version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:               flagNameLicense,
				Aliases:            []string{"l"},
				Value:              false,
				DisableDefaultText: true,
				Usage:              "show licensesa.",
			},
			&cli.BoolFlag{
				Name:               flagNameNeoVim,
				Value:              false,
				DisableDefaultText: true,
				Usage:              "use NeoVim.",
			},
			&cli.BoolFlag{
				Name:               flagNameNoCdr,
				Value:              false,
				DisableDefaultText: true,
				Usage:              "disable clipboard-data-receiver.",
			},
			&cli.BoolFlag{
				Name:               flagNameNoPf,
				Value:              false,
				DisableDefaultText: true,
				Usage:              "disable port-forwarder.",
			},
			&cli.BoolFlag{
				Name:               flagNameNoTmux,
				Value:              false,
				DisableDefaultText: true,
				Usage:              "disable tmux.",
			},
			&cli.StringFlag{
				Name:  flagNameShell,
				Value: "",
				Usage: "start with shell.",
			},
		},
		Action: func(cCtx *cli.Context) error {
			// If the license flag is set, display the license and exit
			if cCtx.Bool(flagNameLicense) {
				fmt.Println(license)
				fmt.Println()
				fmt.Println(notice)
				os.Exit(0)
			}

			cli.ShowAppHelp(cCtx)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:            "run",
				Usage:           "Run container use `docker run`",
				UsageText:       "devcontainer.vim run [DOCKER_OPTIONS...] [DOCKER_ARGS...]",
				HideHelp:        true,
				SkipFlagParsing: true,
				Action: func(cCtx *cli.Context) error {
					// Set up a container with docker run

					// Check requirements
					// 1. docker
					isExistsDocker := util.IsExistsCommand(containerCommand)
					if !isExistsDocker {
						fmt.Fprintf(os.Stderr, "docker not found.")
						os.Exit(1)
					}

					// Determine shell usage
					shell := ""
					if cCtx.String(flagNameShell) != "" {
						shell = cCtx.String(flagNameShell)
					} else if os.Getenv(envDevcontainerShellType) != "" {
						shell = os.Getenv(envDevcontainerShellType)
					}

					// Determine cdr usage
					noCdr := false
					if cCtx.String(flagNameNoCdr) != "" {
						noCdr = cCtx.Bool(flagNameNoCdr)
					}

					// Determine port-forwarder usage
					noPf := false
					if cCtx.String(flagNameNoPf) != "" {
						noPf = cCtx.Bool(flagNameNoPf)
					}

					// Determine tmux usage
					noTmux := false
					if cCtx.String(flagNameNoTmux) != "" {
						noTmux = cCtx.Bool(flagNameNoTmux)
					}

					// Download necessary files

					nvim := false
					if cCtx.Bool(flagNameNeoVim) || os.Getenv(envDevcontainerVimType) == "nvim" {
						nvim = true
					}
					cdrPath, err := tools.InstallRunTools(binDir, nvim)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error installing run tools: %v\n", err)
						os.Exit(1)
					}

					// Get default arguments
					defaultRunargsBytes, err := os.ReadFile(runargs)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error reading runargs: %v\n", err)
						os.Exit(1)
					}
					defaultRunargsString := string(defaultRunargsBytes)

					if runtime.GOOS == "windows" {
						// Start container
						// Windows does not support shell variable expansion well, so runargs is not used
						err = devcontainer.Run(cCtx.Args().Slice(), noCdr, noPf, noTmux, cdrPath, binDir, nvim, shell, configDirForDocker, vimrc, []string{})
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error running docker: %v\n", err)
							os.Exit(1)
						}
					} else {
						// Expand shell variables in default arguments
						extractedDofaultRunargsString, err := util.ExtractShellVariables(defaultRunargsString)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error extracting shell variables: %v\n", err)
							os.Exit(1)
						}

						// Split the expanded string into an array
						defaultRunargs, err := shlex.Split(extractedDofaultRunargsString, true)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error splitting runargs: %v\n", err)
							os.Exit(1)
						}

						// Start container
						args := cCtx.Args().Slice()
						if len(args) == 0 {
							fmt.Fprintf(os.Stderr, "Usage: devcontainer.vim run <IMAGE_OR_CONTAINER>\n")
							os.Exit(1)
						}
						err = devcontainer.Run(cCtx.Args().Slice(), noCdr, noPf, noTmux, cdrPath, binDir, nvim, shell, configDirForDocker, vimrc, defaultRunargs)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error running docker: %v\n", err)
							os.Exit(1)
						}
					}

					return nil
				},
			},
			{
				Name:      "templates",
				Usage:     "Run `devcontainer templates`",
				UsageText: "devcontainer.vim templates [DEVCONTAINER_OPTIONS...] WORKSPACE_FOLDER",
				Subcommands: []*cli.Command{
					{
						Name:      "apply",
						Usage:     "Apply template.",
						UsageText: "devcontainer.vim templates apply WORKSPACE_FOLDER",
						Action: func(cCtx *cli.Context) error {

							// Download the list of templates
							indexFileName := "devcontainer-index.json"
							indexFile := filepath.Join(appCacheDir, indexFileName)
							if !util.IsExists(indexFile) {
								fmt.Println("Download template index ... ")
								err := oras.Pull("ghcr.io/devcontainers/index", "latest", appCacheDir)
								if err != nil {
									fmt.Fprintf(os.Stderr, "Error downloading template index: %v\n", err)
									os.Exit(1)
								}
								fmt.Println("done.")
							}

							var indexRoot IndexRoot
							jsonFile := filepath.Join(appCacheDir, indexFileName)
							jsonData, err := os.ReadFile(jsonFile)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error reading template index: %v\n", err)
								os.Exit(1)
							}
							err = json.Unmarshal(jsonData, &indexRoot)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error unmarshalling template index: %v\n", err)
								os.Exit(1)
							}

							var availableTemplateItems []AvailableTemplateItem
							for _, collection := range indexRoot.Collections {
								availableTemplateItems = append(availableTemplateItems, collection.Templates...)
							}

							names := []string{}
							for _, item := range availableTemplateItems {
								names = append(names, item.Name)
							}

							prompt := promptui.Select{
								Label:             "Select Template",
								Items:             names,
								StartInSearchMode: true,
								Searcher: func(input string, index int) bool {
									item := names[index]
									name := strings.Replace(strings.ToLower(item), " ", "", -1)
									input = strings.Replace(strings.ToLower(input), " ", "", -1)

									return strings.Contains(name, input)
								},
							}

							i, _, err := prompt.Run()
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error running prompt: %v\n", err)
								os.Exit(1)
							}

							selectedItem := availableTemplateItems[i]

							// Execute the devcontainer template subcommand

							// Download necessary files
							devcontainerFilePath, err := tools.InstallTemplatesTools(binDir)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error installing template tools: %v\n", err)
								os.Exit(1)
							}

							// Use the end of the command line arguments as the value for --workspace-folder
							args := cCtx.Args().Slice()
							if len(args) == 0 {
								fmt.Fprintf(os.Stderr, "Error: missing workspace folder.\n")
								fmt.Fprintf(os.Stderr, "Usage: devcontainer.vim templates apply <WORKSPACE_FOLDER>\n")
								os.Exit(1)
							}
							workspaceFolder := args[len(args)-1]

							// Resolve to absolute path to support both relative and absolute paths
							absWorkspaceFolder, err := filepath.Abs(workspaceFolder)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error resolving workspace folder path: %v\n", err)
								os.Exit(1)
							}
							info, err := os.Stat(absWorkspaceFolder)
							if err != nil {
								if errors.Is(err, os.ErrNotExist) {
									fmt.Fprintf(os.Stderr, "Error: workspace folder does not exist: %s\n", workspaceFolder)
								} else {
									fmt.Fprintf(os.Stderr, "Error accessing workspace folder: %v\n", err)
								}
								os.Exit(1)
							}
							if !info.IsDir() {
								fmt.Fprintf(os.Stderr, "Error: workspace folder is not a directory: %s\n", workspaceFolder)
								os.Exit(1)
							}
							workspaceFolder = absWorkspaceFolder

							templateID := selectedItem.ID + ":" + selectedItem.Version

							// Start the container using devcontainer
							output, err := devcontainer.Templates(
								devcontainerFilePath,
								workspaceFolder,
								templateID)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error applying template: %v\n", err)
								os.Exit(1)
							}

							fmt.Println(output)

							return nil
						},
					},
				},
			},
			{
				Name:            "start",
				Usage:           "Run `devcontainer up` and `devcontainer exec`",
				UsageText:       "devcontainer.vim start [DEVCONTAINER_OPTIONS...] WORKSPACE_FOLDER",
				HideHelp:        true,
				SkipFlagParsing: true,
				Action: func(cCtx *cli.Context) error {
					// Set up the container with devcontainer

					// Determine shell usage
					shell := ""
					if cCtx.String(flagNameShell) != "" {
						shell = cCtx.String(flagNameShell)
					} else if os.Getenv(envDevcontainerShellType) != "" {
						shell = os.Getenv(envDevcontainerShellType)
					}

					// Determine cdr usage
					noCdr := false
					if cCtx.String(flagNameNoCdr) != "" {
						noCdr = cCtx.Bool(flagNameNoCdr)
					}

					// Determine port-forwarder usage
					noPf := false
					if cCtx.String(flagNameNoCdr) != "" {
						noPf = cCtx.Bool(flagNameNoPf)
					}

					// Determine tmux usage
					noTmux := false
					if cCtx.String(flagNameNoTmux) != "" {
						noTmux = cCtx.Bool(flagNameNoTmux)
					}

					// Download necessary files
					nvim := false
					if cCtx.Bool(flagNameNeoVim) || os.Getenv(envDevcontainerVimType) == "nvim" {
						nvim = true
					}
					devcontainerPath, cdrPath, err := tools.InstallStartTools(tools.DefaultInstallerUseServices{}, binDir)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error installing start tools: %v\n", err)
						os.Exit(1)
					}

					// Use the end of the command line arguments as the value for --workspace-folder
					args := cCtx.Args().Slice()
					if len(args) == 0 {
						fmt.Fprintf(os.Stderr, "Error: missing workspace folder.\n")
						fmt.Fprintf(os.Stderr, "Usage: devcontainer.vim start <WORKSPACE_FOLDER>\n")
						os.Exit(1)
					}
					workspaceFolder := args[len(args)-1]
					configFilePath, dereferencedMounts, err := devcontainer.CreateConfigFile(devcontainerPath, workspaceFolder, configDirForDevcontainer)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							fmt.Fprintf(os.Stderr, "Configuration file not found: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error creating config file: %v\n", err)
						}
						os.Exit(1)
					}

					// Start the container using devcontainer
					err = devcontainer.Start(devcontainer.DefaultDevcontainerStartUseService{}, args, devcontainerPath, noCdr, noPf, noTmux, cdrPath, binDir, nvim, shell, configFilePath, vimrc, dereferencedMounts)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error executing devcontainer: %v\n", err)
						}
						os.Exit(1)
					}

					return nil
				},
			},
			{
				Name:            "stop",
				Usage:           "Stop devcontainers.",
				UsageText:       "devcontainer.vim stop WORKSPACE_FOLDER",
				HideHelp:        true,
				SkipFlagParsing: true,
				Action: func(cCtx *cli.Context) error {
					// Set up the container with devcontainer

					// Download necessary files
					devcontainerPath, err := tools.InstallStopTools(binDir)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							fmt.Fprintf(os.Stderr, "Configuration file not found: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error installing stop tools: %v\n", err)
						}
						os.Exit(1)
					}

					// Stop the container using devcontainer
					err = devcontainer.Stop(cCtx.Args().Slice(), devcontainerPath, configDirForDevcontainer)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error stopping devcontainer: %v\n", err)
						}
						os.Exit(1)
					}

					fmt.Printf("Stop containers\n")

					return nil
				},
			},
			{
				Name:            "down",
				Usage:           "Stop and remove devcontainers.",
				UsageText:       "devcontainer.vim down WORKSPACE_FOLDER",
				HideHelp:        true,
				SkipFlagParsing: true,
				Action: func(cCtx *cli.Context) error {
					// Set up the container with devcontainer

					// Download necessary files
					devcontainerPath, err := tools.InstallDownTools(binDir)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							fmt.Fprintf(os.Stderr, "Configuration file not found: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error installing down tools: %v\n", err)
						}
						os.Exit(1)
					}

					// Stop the container using devcontainer
					err = devcontainer.Down(cCtx.Args().Slice(), devcontainerPath, configDirForDevcontainer)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error downing devcontainer: %v\n", err)
						}
						os.Exit(1)
					}

					// Remove configuration files
					// Use the end of the command line arguments as the value for --workspace-folder
					args := cCtx.Args().Slice()
					workspaceFolder := args[len(args)-1]
					configDir, err := util.GetConfigDir(appCacheDir, workspaceFolder)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error getting configuration file path: %v\n", err)
						}
						os.Exit(1)
					}

					fmt.Printf("Remove configuration file: `%s`\n", configDir)
					err = os.RemoveAll(configDir)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error removing configuration file: %v\n", err)
						}
						os.Exit(1)
					}

					return nil
				},
			},
			{
				Name:            "config",
				Usage:           "devcontainer.vim's config information.",
				UsageText:       "devcontainer.vim config [OPTIONS...]",
				HideHelp:        false,
				SkipFlagParsing: false,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    flagNameGenerate,
						Aliases: []string{"g"},
						Value:   false,
						Usage:   "generate sample config file.",
					},
					&cli.StringFlag{
						Name:    flagNameHome,
						Aliases: []string{},
						Value:   "/home/vscode",
						Usage:   "generate sample config's home directory.",
					},
					&cli.StringFlag{
						Name:    flagNameOutput,
						Aliases: []string{"o"},
						Value:   ".devcontainer/devcontainer.vim.json",
						Usage:   "generate sample config output file path.",
					},
				},
				Action: func(cCtx *cli.Context) error {
					// If non-option arguments are passed, output help and exit
					if cCtx.NumFlags() == 0 || cCtx.Args().Present() {
						cli.ShowSubcommandHelpAndExit(cCtx, 0)
					}

					// If the generate flag is set, output a template for the configuration file
					if cCtx.Bool(flagNameGenerate) {

						// Use the value specified by the home option to replace the bind destination
						devcontainerVimJSON := strings.Replace(devcontainerVimJSONTemplate, "{{ remoteEnv:HOME }}", cCtx.String(flagNameHome), -1)

						if cCtx.IsSet(flagNameOutput) {
							// If the output option is specified, output to the specified path
							configFilePath := cCtx.String(flagNameOutput)

							// Create the output directory
							err := os.MkdirAll(filepath.Dir(configFilePath), 0766)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
								os.Exit(1)
							}

							// Output the configuration file sample
							err = os.WriteFile(configFilePath, []byte(devcontainerVimJSON), 0666)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error writing config file: %v\n", err)
								os.Exit(1)
							}
						} else {
							// If the output option is not specified, output to standard output
							fmt.Print(devcontainerVimJSON)
						}
					}

					return nil
				},
			},
			{
				Name:            "vimrc",
				Usage:           "devcontainer.vim's vimrc information.",
				UsageText:       "devcontainer.vim vimrc [OPTIONS...]",
				HideHelp:        false,
				SkipFlagParsing: false,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    flagNameGenerate,
						Aliases: []string{"g"},
						Value:   false,
						Usage:   "regenerate vimrc file.",
					},
					&cli.BoolFlag{
						Name:    flagNameOpen,
						Aliases: []string{"o"},
						Value:   false,
						Usage:   "open and display vimrc.",
					},
				},
				Action: func(cCtx *cli.Context) error {
					// If non-option arguments are passed, output help and exit
					if cCtx.NumFlags() == 0 || cCtx.Args().Present() {
						cli.ShowSubcommandHelpAndExit(cCtx, 0)
					}

					// If the generate flag is set, regenerate vimrc
					if cCtx.Bool(flagNameGenerate) {
						err := os.WriteFile(vimrc, []byte(additionalVimrc), 0666)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error writing vimrc: %v\n", err)
							os.Exit(1)
						}
						fmt.Printf("Generated additional vimrc to: %s\n", vimrc)
					}

					if cCtx.Bool(flagNameOpen) {
						err := util.OpenFileWithDefaultApp(vimrc)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error opening vimrc: %v\n", err)
							os.Exit(1)
						}
						fmt.Printf("%s\n", vimrc)
					}

					return nil
				},
			},
			{
				Name:            "runargs",
				Usage:           "run subcommand's default arguments.",
				UsageText:       "devcontainer.vim runargs [OPTIONS...]",
				HideHelp:        false,
				SkipFlagParsing: false,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    flagNameGenerate,
						Aliases: []string{"g"},
						Value:   false,
						Usage:   "regenerate runargs file.",
					},
					&cli.BoolFlag{
						Name:    flagNameOpen,
						Aliases: []string{"o"},
						Value:   false,
						Usage:   "open and display runargs.",
					},
				},
				Action: func(cCtx *cli.Context) error {
					// If non-option arguments are passed, output help and exit
					if cCtx.NumFlags() == 0 || cCtx.Args().Present() {
						cli.ShowSubcommandHelpAndExit(cCtx, 0)
					}

					// If the generate flag is set, regenerate runargs
					if cCtx.Bool(flagNameGenerate) {
						err := os.WriteFile(runargs, []byte(runargsContent), 0666)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error writing runargs: %v\n", err)
							os.Exit(1)
						}
						fmt.Printf("Generated additional runargs to: %s\n", runargs)
					}

					if cCtx.Bool(flagNameOpen) {
						err := util.OpenFileWithDefaultApp(runargs)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error opening runargs: %v\n", err)
							os.Exit(1)
						}
						fmt.Printf("%s\n", runargs)
					}

					return nil
				},
			},
			{
				Name:            "tool",
				Usage:           "Management tools",
				UsageText:       "devcontainer.vim tool SUB_COMMAND",
				HideHelp:        false,
				SkipFlagParsing: false,
				Subcommands: []*cli.Command{
					{
						Name:            "vim",
						Usage:           "Management vim",
						UsageText:       "devcontainer.vim tool vim SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly vim",
								UsageText:       "devcontainer.vim tool vim download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  flagNameArch,
										Value: runtime.GOARCH,
										Usage: "download cpu archtecture.",
									},
								},
								Action: func(cCtx *cli.Context) error {

									// Download Vim
									_, err := tools.VIM(tools.DefaultInstallerUseServices{}).Install(binDir, cCtx.String(flagNameArch), true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing vim: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:            "nvim",
						Usage:           "Management nvim",
						UsageText:       "devcontainer.vim tool nvim SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly nvim",
								UsageText:       "devcontainer.vim tool nvim download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  flagNameArch,
										Value: runtime.GOARCH,
										Usage: "download cpu archtecture.",
									},
								},
								Action: func(cCtx *cli.Context) error {

									// Download NeoVim
									_, err := tools.NVIM(tools.DefaultInstallerUseServices{}).Install(binDir, cCtx.String(flagNameArch), true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing nvim: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:            "tmux",
						Usage:           "Management tmux",
						UsageText:       "devcontainer.vim tool tmux SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly tmux",
								UsageText:       "devcontainer.vim tool tmux download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  flagNameArch,
										Value: runtime.GOARCH,
										Usage: "download cpu archtecture.",
									},
								},
								Action: func(cCtx *cli.Context) error {

									// Download tmux
									_, err := tools.Tmux(tools.DefaultInstallerUseServices{}).Install(binDir, cCtx.String(flagNameArch), true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing tmux: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:            "devcontainer",
						Usage:           "Management devcontainer cli",
						UsageText:       "devcontainer.vim tool devcontainer SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly devcontainer cli",
								UsageText:       "devcontainer.vim tool devcontainer download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Action: func(cCtx *cli.Context) error {

									// Download devcontainer
									_, err := tools.DEVCONTAINER(tools.DefaultInstallerUseServices{}).Install(binDir, "", true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing devcontainer: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:            tools.CdrFileName,
						Usage:           "Management clipboard-data-receiver",
						UsageText:       "devcontainer.vim tool clipboard-data-receiver SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly clipboard-data-receiver cli",
								UsageText:       "devcontainer.vim tool clipboard-data-receiver download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Action: func(cCtx *cli.Context) error {

									// Download clipboard-data-receiver
									_, err := tools.CDR(tools.DefaultInstallerUseServices{}).Install(binDir, "", true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing clipboard-data-receiver: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:            "port-forwarder",
						Usage:           "Management port-forwarder on container",
						UsageText:       "devcontainer.vim tool port-forwarder SUB_COMMAND",
						HideHelp:        false,
						SkipFlagParsing: false,
						Subcommands: []*cli.Command{
							{
								Name:            "download",
								Usage:           "Download newly port-forwarder cli",
								UsageText:       "devcontainer.vim tool port-forwarder download",
								HideHelp:        false,
								SkipFlagParsing: false,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  flagNameArch,
										Value: runtime.GOARCH,
										Usage: "download cpu archtecture.",
									},
								},
								Action: func(cCtx *cli.Context) error {

									// Download port-forwarder
									_, err := tools.PortForwarderContainer(tools.DefaultInstallerUseServices{}).Install(binDir, cCtx.String(flagNameArch), true)
									if err != nil {
										fmt.Fprintf(os.Stderr, "Error installing port-forwarder: %v\n", err)
										os.Exit(1)
									}

									return nil
								},
							},
						},
					},
				},
			},
			{
				Name:      "clean",
				Usage:     "clean workspace cache files.",
				UsageText: "devcontainer.vim clean",
				Action: func(cCtx *cli.Context) error {

					// Confirmation of execution
					var input string
					fmt.Printf("Do you want to delete all workspace caches? [y/n] > ")
					fmt.Scan(&input)
					input = strings.TrimSpace(input)
					input = strings.ToLower(input)
					if input == "n" || input == "no" {
						return nil
					}

					// Deletion process
					err := os.RemoveAll(configDirForDocker)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error removing docker config directory: %v\n", err)
						}
						os.Exit(1)
					}
					err = os.RemoveAll(configDirForDevcontainer)
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error removing devcontainer config directory: %v\n", err)
						}
						os.Exit(1)
					}

					return nil
				},
			},
			{
				Name:            "index",
				Usage:           "Management dev container template index file",
				UsageText:       "devcontainer.vim index SUB_COMMAND",
				HideHelp:        false,
				SkipFlagParsing: false,
				Subcommands: []*cli.Command{
					{
						Name:      "update",
						Usage:     "Download newly index file",
						UsageText: "devcontainer.vim index update",
						Action: func(cCtx *cli.Context) error {

							// Download the list of templates
							err := oras.Pull("ghcr.io/devcontainers/index", "latest", appCacheDir)
							if err != nil {
								if errors.Is(err, os.ErrNotExist) {
									fmt.Fprintf(os.Stderr, "Index file not found: %v\n", err)
								} else {
									fmt.Fprintf(os.Stderr, "Error updating index: %v\n", err)
								}
								os.Exit(1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:      "self-update",
				Usage:     "Update devcontainer.vim itself",
				UsageText: "devcontainer.vim self-update",
				Action: func(cCtx *cli.Context) error {
					err := tools.SelfUpdate(tools.DefaultInstallerUseServices{})
					if err != nil {
						if errors.Is(err, os.ErrPermission) {
							fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "Error updating devcontainer.vim: %v\n", err)
						}
						os.Exit(1)
					}
					return nil
				},
			},
			{
				Name:      "bash-complete-func",
				Usage:     "Show bash complete func",
				UsageText: "devcontainer.vim bash-complete-func",
				Action: func(cCtx *cli.Context) error {
					fmt.Print(bash_complete_func)
					return nil
				},
			},
		},
	})

	// Run application
	err = devcontainerVimArgProcess.Run(os.Args)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "Permission error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error running devcontainer.vim: %v\n", err)
		}
		os.Exit(1)
	}
}
