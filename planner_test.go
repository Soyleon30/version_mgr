package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanClassifiesFilesIntoNearestTSTarget(t *testing.T) {
	root := t.TempDir(); target := filepath.Join(root, "TS202610月版1次", "TS20261001"); source := filepath.Join(target, "应用A", "第1批", "版本说明")
	writeTestFile(t, filepath.Join(source, "版本说明书-应用A.docx"), "version"); writeTestFile(t, filepath.Join(source, "投产确认书-应用A.docx"), "confirm"); writeTestFile(t, filepath.Join(source, "执行脚本.sql"), "sql")
	plan, err := BuildPlan(root); if err != nil { t.Fatal(err) }; if plan.SourceDirectoryCount != 1 || plan.CopyCount() != 3 { t.Fatalf("unexpected plan: %+v", plan) }
	want := map[string]bool{filepath.Join(target, "版本说明书", "版本说明书-应用A.docx"): false, filepath.Join(target, "投产确认书", "投产确认书-应用A.docx"): false, filepath.Join(target, "执行脚本.sql"): false}
	for _, op := range plan.Operations { if _, ok := want[op.Destination]; !ok { t.Fatalf("unexpected destination %q", op.Destination) }; want[op.Destination] = true }
	for path, found := range want { if !found { t.Errorf("missing destination %q", path) } }
}

func TestBuildPlanKeepsDifferentSameNameFiles(t *testing.T) {
	root := t.TempDir(); target := filepath.Join(root, "TS202610月版1次", "TS20261001")
	writeTestFile(t, filepath.Join(target, "应用A", "第1批", "版本说明", "版本说明书.docx"), "first"); writeTestFile(t, filepath.Join(target, "应用B", "第2批", "版本说明", "版本说明书.docx"), "second")
	plan, err := BuildPlan(root); if err != nil { t.Fatal(err) }; if plan.CopyCount() != 2 { t.Fatalf("copy count = %d, want 2", plan.CopyCount()) }
	wantSecond := filepath.Join(target, "版本说明书", "版本说明书 (2).docx"); if plan.Operations[1].Destination != wantSecond { t.Fatalf("second destination = %q, want %q", plan.Operations[1].Destination, wantSecond) }
}

func TestExecuteAndSecondRunAreIdempotent(t *testing.T) {
	root := t.TempDir(); target := filepath.Join(root, "TS202610月版1次", "TS20261001"); source := filepath.Join(target, "应用A", "第1批", "版本说明", "投产确认书.docx"); writeTestFile(t, source, "same content")
	firstPlan, err := BuildPlan(root); if err != nil { t.Fatal(err) }; result := Execute(firstPlan); if result.Copied != 1 || len(result.Errors) != 0 { t.Fatalf("unexpected execution result: %+v", result) }
	secondPlan, err := BuildPlan(root); if err != nil { t.Fatal(err) }; if secondPlan.CopyCount() != 0 || secondPlan.SkipCount() != 1 { t.Fatalf("second plan is not idempotent: %+v", secondPlan) }
}

func TestBuildPlanReportsSourceWithoutTSTarget(t *testing.T) {
	root := t.TempDir(); writeTestFile(t, filepath.Join(root, "普通目录", "第1批", "版本说明", "文件.txt"), "data"); plan, err := BuildPlan(root); if err != nil { t.Fatal(err) }; if len(plan.Issues) != 1 || plan.CopyCount() != 0 { t.Fatalf("unexpected plan: %+v", plan) }
}

func writeTestFile(t *testing.T, path, content string) { t.Helper(); if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }; if err := os.WriteFile(path, []byte(content), 0o644); err != nil { t.Fatal(err) } }
