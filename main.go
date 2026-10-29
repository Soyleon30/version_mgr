package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() { os.Exit(run()) }

func run() int {
	rootFlag := flag.String("root", "", "投产根目录，例如 D:\\20261009投产")
	dryRun := flag.Bool("dry-run", false, "仅显示复制计划，不写入文件")
	yes := flag.Bool("yes", false, "跳过执行前的确认")
	flag.Parse()

	root := strings.Trim(strings.TrimSpace(*rootFlag), "\"")
	if root == "" {
		fmt.Print("请输入投产根目录: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && len(line) == 0 {
			fmt.Fprintf(os.Stderr, "读取目录失败: %v\n", err)
			return 2
		}
		root = strings.Trim(strings.TrimSpace(line), "\"")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "目录无效: %v\n", err)
		return 2
	}
	plan, err := BuildPlan(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描失败: %v\n", err)
		return 1
	}
	printPlan(plan)
	if plan.CopyCount() == 0 {
		fmt.Println("没有需要复制的文件。")
		return 0
	}
	if *dryRun {
		fmt.Println("当前为预览模式，未修改任何文件。")
		return 0
	}
	if !*yes && !confirm() {
		fmt.Println("已取消，未修改任何文件。")
		return 0
	}
	result := Execute(plan)
	fmt.Printf("\n执行完成：复制 %d，跳过 %d，失败 %d。\n", result.Copied, result.Skipped, len(result.Errors))
	for _, execErr := range result.Errors {
		fmt.Fprintf(os.Stderr, "[失败] %s -> %s: %v\n", execErr.Operation.Source, execErr.Operation.Destination, execErr.Err)
	}
	if len(result.Errors) > 0 { return 1 }
	return 0
}

func printPlan(plan Plan) {
	fmt.Printf("扫描到 %d 个“版本说明”目录。\n", plan.SourceDirectoryCount)
	for _, issue := range plan.Issues { fmt.Printf("[忽略] %s: %s\n", issue.Path, issue.Reason) }
	for _, op := range plan.Operations {
		label := "复制"
		if op.Action == ActionSkip { label = "跳过" }
		fmt.Printf("[%s] %s\n       -> %s", label, op.Source, op.Destination)
		if op.Reason != "" { fmt.Printf(" (%s)", op.Reason) }
		fmt.Println()
	}
	fmt.Printf("计划：复制 %d，跳过 %d，忽略 %d。\n", plan.CopyCount(), plan.SkipCount(), len(plan.Issues))
}

func confirm() bool {
	fmt.Print("确认执行？[y/N]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes" || answer == "是"
}
