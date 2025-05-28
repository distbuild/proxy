package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendPathToInputFiles(t *testing.T) {
	dir := "/home/user/project"
	inputFiles := []string{"file1.txt", "file2.txt"}
	includePath := []string{"include1", "include2"}

	// Mock the directory structure for testing
	err := os.MkdirAll(filepath.Join(dir, "include1"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	err = os.MkdirAll(filepath.Join(dir, "include2"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err = os.WriteFile(filepath.Join(dir, "include1", "file3.txt"), nil, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, "include2", "file4.txt"), nil, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	newInputFiles, err := appendPathToInputFiles(dir, inputFiles, includePath)
	if err != nil {
		t.Fatalf("Failed to append paths to input files: %v", err)
	}

	expectedInputFiles := []string{"file1.txt", "file2.txt", filepath.FromSlash("include1/file3.txt"), filepath.FromSlash("include2/file4.txt")}
	assert.Equal(t, expectedInputFiles, newInputFiles)

	_ = os.RemoveAll(dir)
}

func TestCompileDependency(t *testing.T) {
	// Create a temporary JSON file for testing
	dir := filepath.FromSlash("./testdata")
	jsonFile := "compile_info.json"

	compileInfo := CompileInfo{
		Commands: []Command{
			{
				Command:      "gcc",
				CompilerType: "gcc",
				InputFiles:   []string{"file1.c", "file2.c"},
				OutputFile:   "output.o",
				Includes:     []string{"include1", "include2"},
				Module:       "test_module",
			},
		},
	}

	err := os.MkdirAll(filepath.Join(dir, "out"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(dir, "out", jsonFile), []byte{}, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	compileInfoBytes, err := json.Marshal(compileInfo)
	if err != nil {
		t.Fatalf("Failed to parse data: %v", err)
	}

	err = os.WriteFile(filepath.Join(dir, "out", jsonFile), compileInfoBytes, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to write JSON file: %v", err)
	}

	err = os.MkdirAll(filepath.Join(dir, "include1"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	err = os.MkdirAll(filepath.Join(dir, "include2"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err = os.WriteFile(filepath.Join(dir, "include1", "file3.c"), nil, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, "include2", "file4.c"), nil, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	tasks, err := CompileDependency(dir, "compile_info.json")
	if err != nil {
		t.Fatalf("Failed to compile dependency: %v", err)
	}

	expectedTasks := []BuildInfo{
		{
			BuildRule:    "gcc",
			BuildFiles:   []string{"file1.c", "file2.c", filepath.FromSlash("include1/file3.c"), filepath.FromSlash("include2/file4.c")},
			BuildTargets: []string{"output.o"},
		},
	}

	assert.Equal(t, expectedTasks, tasks)
	_ = os.RemoveAll(dir)
}
