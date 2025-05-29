package task

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

type Command struct {
	Command      string   `json:"command"`
	CompilerType string   `json:"compilerType"`
	InputFiles   []string `json:"inputFiles"`
	OutputFile   string   `json:"outputFile"`
	Includes     []string `json:"includes"`
	Module       string   `json:"module"`
}

type CompileInfo struct {
	Commands []Command `json:"commands"`
}

type BuildInfo struct {
	BuildRule    string
	BuildFiles   []string
	BuildTargets []string
}

// Symlink or not
func isSymlink(path string) (bool, error) {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.Mode()&os.ModeSymlink != 0, nil
}

// resolveSymlink and get rel path
func resolveSymlink(path string) (string, error) {
	isLink, err := isSymlink(path)
	if err != nil {
		return "", err
	}
	if isLink {
		targetPath, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		fileinfo, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(path), targetPath))
		if err != nil {
			return "", nil
		}

		if fileinfo == "" {
			return "", nil
		}

		_, err = os.Stat(fileinfo)
		if err != nil {
			return "", nil
		} else {
			return fileinfo, nil
		}
	}

	return path, nil
}

func appendPathToInputFiles(dir string, inputFiles []string, includePath []string) ([]string, error) {

	var walkDir func(path string, linkPath string) error
	walkDir = func(path string, linkPath string) error {
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("fail to read dir %s: %v", path, err)
		}

		for _, entry := range entries {
			entryPath := filepath.Join(path, entry.Name())
			linkEntryPath := filepath.Join(linkPath, entry.Name())

			isLink, err := isSymlink(entryPath)
			if err != nil {
				return fmt.Errorf("fail to check symlink %s: %v", entryPath, err)
			}

			if isLink {
				// is symlink: resolve real path
				resolvedPath, err := resolveSymlink(entryPath)
				if err != nil {
					return fmt.Errorf("fail to resolve symlink %s: %v", entryPath, err)
				}

				if resolvedPath == "" {
					continue
				}

				resolvedInfo, err := os.Stat(resolvedPath)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("resolved symlink target path:%s not exist", resolvedPath)
					}
					return fmt.Errorf("fail to get resolved symlink target path %s: %v", resolvedPath, err)
				}

				if resolvedInfo.IsDir() {
					// recursive traversal dir
					if err := walkDir(resolvedPath, linkEntryPath); err != nil {
						return err
					}
				} else {
					// add target file to inputFiles
					relativeFilePath, err := filepath.Rel(dir, linkEntryPath)
					if err != nil {
						return fmt.Errorf("fail to get relative path for %s: %v", linkEntryPath, err)
					}
					if len(relativeFilePath) > 0 && !slices.Contains(inputFiles, relativeFilePath) {
						inputFiles = append(inputFiles, relativeFilePath)
					}
				}
			} else {
				// not symlink
				fileInfo, err := os.Stat(entryPath)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("path:%s not exist", entryPath)
					}
					return fmt.Errorf("fail to get path %s: %v", entryPath, err)
				}

				if fileInfo.IsDir() {
					// recursive traversal dir
					if err := walkDir(entryPath, linkEntryPath); err != nil {
						return err
					}
				} else {
					// add target file to inputFiles
					relativeFilePath, err := filepath.Rel(dir, linkEntryPath)
					if err != nil {
						return fmt.Errorf("fail to get relative path for %s: %v", linkEntryPath, err)
					}
					if len(relativeFilePath) > 0 && !slices.Contains(inputFiles, relativeFilePath) {
						inputFiles = append(inputFiles, relativeFilePath)
					}
				}
			}
		}
		return nil
	}

	// Traverse all inclusive items
	for _, item := range includePath {
		include := filepath.Join(dir, item)

		isLink, err := isSymlink(include)
		if err != nil {
			if os.IsNotExist(err) {
				return inputFiles, fmt.Errorf("path:%s not exist", include)
			}
			return inputFiles, fmt.Errorf("fail to check symlink %s: %v", include, err)
		}

		if isLink {
			resolvedPath, err := resolveSymlink(include)
			if err != nil {
				return inputFiles, fmt.Errorf("fail to resolve symlink %s: %v", include, err)
			}

			if resolvedPath == "" {
				continue
			}

			resolvedInfo, err := os.Stat(resolvedPath)
			if err != nil {
				if os.IsNotExist(err) {
					return inputFiles, fmt.Errorf("resolved symlink target path:%s not exist", resolvedPath)
				}
				return inputFiles, fmt.Errorf("fail to get resolved symlink target path %s: %v", resolvedPath, err)
			}

			if resolvedInfo.IsDir() {
				if err := walkDir(resolvedPath, include); err != nil {
					return inputFiles, fmt.Errorf("error to traverse '%s': %v", resolvedPath, err)
				}
			} else {
				relativeFilePath, err := filepath.Rel(dir, include)
				if err != nil {
					return inputFiles, fmt.Errorf("fail to get relative path for %s: %v", include, err)
				}
				if len(relativeFilePath) > 0 && !slices.Contains(inputFiles, relativeFilePath) {
					inputFiles = append(inputFiles, relativeFilePath)
				}
			}
		} else {
			fileInfo, err := os.Stat(include)
			if err != nil {
				if os.IsNotExist(err) {
					return inputFiles, fmt.Errorf("path:%s not exist", include)
				}
				return inputFiles, fmt.Errorf("fail to get path %s: %v", include, err)
			}

			if fileInfo.IsDir() {
				if err := walkDir(include, include); err != nil {
					return inputFiles, fmt.Errorf("error to traverse '%s': %v", include, err)
				}
			} else {
				relativeFilePath, err := filepath.Rel(dir, include)
				if err != nil {
					return inputFiles, fmt.Errorf("fail to get relative path for %s: %v", include, err)
				}
				if len(relativeFilePath) > 0 && !slices.Contains(inputFiles, relativeFilePath) {
					inputFiles = append(inputFiles, relativeFilePath)
				}
			}
		}
	}

	return inputFiles, nil
}

func parseCommand(command, compiletype string) string {
	if compiletype == "clang" || compiletype == "clang++" {
		clangPattern := regexp.MustCompile(`prebuilts/clang/host/linux-x86/clang-[a-zA-Z0-9]+/bin/(clang|clang\+\+)`)
		return clangPattern.ReplaceAllStringFunc(command, func(match string) string {
			return clangPattern.FindStringSubmatch(match)[1]
		})
	}
	return command
}

func CompileDependency(path string, filename string) ([]BuildInfo, error) {
	var tasks []BuildInfo

	filePath := filepath.Join(path, "out", filename)
	log.Printf("Compile JSON: %s\n", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return tasks, fmt.Errorf("failed to open JSON file: %v", err)
	}

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	decoder := json.NewDecoder(file)

	var compileInfo CompileInfo

	err = decoder.Decode(&compileInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	for _, command := range compileInfo.Commands {
		var task BuildInfo

		// command
		task.BuildRule = parseCommand(command.Command, command.CompilerType)

		// targets
		if command.OutputFile != "" {
			task.BuildTargets = append(task.BuildTargets, command.OutputFile)
		}

		// buildFiles
		task.BuildFiles = command.InputFiles
		if len(command.Includes) > 0 {
			task.BuildFiles, err = appendPathToInputFiles(path, task.BuildFiles, command.Includes)
			if err != nil {
				return nil, fmt.Errorf("failed to append include path: %v", err)
			}
		}

		// all tasks
		tasks = append(tasks, task)
	}

	return tasks, nil
}
