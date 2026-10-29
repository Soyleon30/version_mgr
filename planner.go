package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Action int
const ( ActionCopy Action = iota; ActionSkip )

type Operation struct { Source, Destination string; Action Action; Reason string }
type Issue struct { Path, Reason string }
type Plan struct { Root string; SourceDirectoryCount int; Operations []Operation; Issues []Issue }

func (p Plan) CopyCount() int { n := 0; for _, op := range p.Operations { if op.Action == ActionCopy { n++ } }; return n }
func (p Plan) SkipCount() int { return len(p.Operations) - p.CopyCount() }
type reservedFile struct { source string }

func BuildPlan(root string) (Plan, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil { return Plan{}, err }
	if !info.IsDir() { return Plan{}, fmt.Errorf("%s 不是目录", root) }
	plan := Plan{Root: root}
	var sourceDirs []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			plan.Issues = append(plan.Issues, Issue{path, walkErr.Error()})
			if entry != nil && entry.IsDir() { return fs.SkipDir }
			return nil
		}
		if entry.IsDir() && entry.Name() == "版本说明" { sourceDirs = append(sourceDirs, path); return fs.SkipDir }
		return nil
	})
	if err != nil { return Plan{}, err }
	sort.Strings(sourceDirs)
	plan.SourceDirectoryCount = len(sourceDirs)
	reservations := make(map[string]reservedFile)
	for _, sourceDir := range sourceDirs {
		targetDir, ok := nearestTSTarget(root, sourceDir)
		if !ok { plan.Issues = append(plan.Issues, Issue{sourceDir, "向上未找到名称以 TS202 开头的目标目录"}); continue }
		walkErr := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, fileErr error) error {
			if fileErr != nil { plan.Issues = append(plan.Issues, Issue{path, fileErr.Error()}); if entry != nil && entry.IsDir() { return fs.SkipDir }; return nil }
			if entry.IsDir() { return nil }
			fileInfo, infoErr := entry.Info()
			if infoErr != nil { plan.Issues = append(plan.Issues, Issue{path, infoErr.Error()}); return nil }
			if !fileInfo.Mode().IsRegular() { plan.Issues = append(plan.Issues, Issue{path, "不是普通文件"}); return nil }
			destination := desiredDestination(targetDir, entry.Name())
			op, planErr := planOperation(path, destination, reservations)
			if planErr != nil { plan.Issues = append(plan.Issues, Issue{path, planErr.Error()}); return nil }
			plan.Operations = append(plan.Operations, op)
			return nil
		})
		if walkErr != nil { plan.Issues = append(plan.Issues, Issue{sourceDir, walkErr.Error()}) }
	}
	sort.SliceStable(plan.Operations, func(i, j int) bool { return plan.Operations[i].Source < plan.Operations[j].Source })
	return plan, nil
}

func nearestTSTarget(root, sourceDir string) (string, bool) {
	root = filepath.Clean(root)
	current := filepath.Dir(sourceDir)
	for {
		if strings.HasPrefix(strings.ToUpper(filepath.Base(current)), "TS202") { return current, true }
		if samePath(current, root) { break }
		parent := filepath.Dir(current)
		if samePath(parent, current) || !isWithin(root, parent) { break }
		current = parent
	}
	return "", false
}

func desiredDestination(targetDir, sourceName string) string {
	destinationDir := targetDir
	if strings.HasPrefix(sourceName, "版本说明书") { destinationDir = filepath.Join(targetDir, "版本说明书") }
	if strings.HasPrefix(sourceName, "投产确认书") { destinationDir = filepath.Join(targetDir, "投产确认书") }
	return filepath.Join(destinationDir, sourceName)
}

func planOperation(source, desired string, reservations map[string]reservedFile) (Operation, error) {
	for number, candidate := 1, desired; ; number, candidate = number+1, suffixedPath(desired, number+1) {
		key := canonicalPath(candidate)
		if reserved, exists := reservations[key]; exists {
			equal, err := filesEqual(source, reserved.source); if err != nil { return Operation{}, err }
			if equal { return Operation{source, candidate, ActionSkip, "计划中已有相同内容"}, nil }
			continue
		}
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.Mode().IsRegular():
			equal, compareErr := filesEqual(source, candidate); if compareErr != nil { return Operation{}, compareErr }
			if equal { return Operation{source, candidate, ActionSkip, "目标已有相同内容"}, nil }
			continue
		case err == nil: continue
		case errors.Is(err, os.ErrNotExist):
			reservations[key] = reservedFile{source}
			reason := ""; if number > 1 { reason = "同名文件已存在，已自动编号" }
			return Operation{source, candidate, ActionCopy, reason}, nil
		default: return Operation{}, err
		}
	}
}

func suffixedPath(path string, number int) string {
	ext := filepath.Ext(path); base := strings.TrimSuffix(filepath.Base(path), ext)
	return filepath.Join(filepath.Dir(path), fmt.Sprintf("%s (%d)%s", base, number, ext))
}
func canonicalPath(path string) string { return strings.ToLower(filepath.Clean(path)) }
func samePath(left, right string) bool { return canonicalPath(left) == canonicalPath(right) }
func isWithin(root, path string) bool { relative, err := filepath.Rel(root, path); if err != nil { return false }; return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) }

func filesEqual(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left); if err != nil { return false, err }
	rightInfo, err := os.Stat(right); if err != nil { return false, err }
	if leftInfo.Size() != rightInfo.Size() { return false, nil }
	leftHash, err := fileHash(left); if err != nil { return false, err }
	rightHash, err := fileHash(right); if err != nil { return false, err }
	return leftHash == rightHash, nil
}
func fileHash(path string) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte; file, err := os.Open(path); if err != nil { return result, err }; defer file.Close()
	hash := sha256.New(); if _, err = io.Copy(hash, file); err != nil { return result, err }; copy(result[:], hash.Sum(nil)); return result, nil
}

type ExecutionError struct { Operation Operation; Err error }
type ExecutionResult struct { Copied, Skipped int; Errors []ExecutionError }
func Execute(plan Plan) ExecutionResult {
	var result ExecutionResult
	for _, op := range plan.Operations {
		if op.Action == ActionSkip { result.Skipped++; continue }
		if err := copyFileAtomically(op.Source, op.Destination); err != nil { result.Errors = append(result.Errors, ExecutionError{op, err}); continue }
		result.Copied++
	}
	return result
}

func copyFileAtomically(source, destination string) error {
	if samePath(source, destination) { return nil }
	if destinationInfo, err := os.Stat(destination); err == nil {
		if destinationInfo.Mode().IsRegular() { equal, compareErr := filesEqual(source, destination); if compareErr != nil { return compareErr }; if equal { return nil } }
		return fmt.Errorf("目标在扫描后被创建或修改，为避免覆盖已停止复制")
	} else if !errors.Is(err, os.ErrNotExist) { return err }
	sourceFile, err := os.Open(source); if err != nil { return err }; defer sourceFile.Close()
	sourceInfo, err := sourceFile.Stat(); if err != nil { return err }
	destinationDir := filepath.Dir(destination); if err = os.MkdirAll(destinationDir, 0o755); err != nil { return err }
	temporary, err := os.CreateTemp(destinationDir, ".versionmgr-*"); if err != nil { return err }
	temporaryPath := temporary.Name(); keepTemporary := false
	defer func() { if !keepTemporary { _ = os.Remove(temporaryPath) } }()
	if _, err = io.Copy(temporary, sourceFile); err != nil { _ = temporary.Close(); return err }
	if err = temporary.Sync(); err != nil { _ = temporary.Close(); return err }
	if err = temporary.Close(); err != nil { return err }
	if err = os.Chmod(temporaryPath, sourceInfo.Mode().Perm()); err != nil { return err }
	if err = os.Chtimes(temporaryPath, time.Now(), sourceInfo.ModTime()); err != nil { return err }
	if err = os.Rename(temporaryPath, destination); err != nil { return err }
	keepTemporary = true
	return nil
}
