package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/goodtiger/openclaw-install/internal/output"
)

var osArchMap = map[string]map[string]string{
	"darwin": {
		"amd64": "darwin_amd64",
		"arm64": "darwin_arm64",
	},
	"linux": {
		"amd64": "linux_amd64",
		"arm64": "linux_arm64",
	},
	"windows": {
		"amd64": "windows_amd64",
		"arm64": "windows_arm64",
	},
}

var githubDownloadURLs = []string{
	"https://github.com",
	"https://ghproxy.com/https://github.com",
}

func runUpgrade(args []string, out, errOut io.Writer) error {
	fs := newFlagSet("upgrade", errOut, "自我更新到最新版本。")
	forceFlag := fs.Bool("force", false, "即使当前版本已是最新也强制升级")
	rollbackFlag := fs.Bool("rollback-on-failure", false, "升级失败时自动回滚到旧版本")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前二进制路径失败: %w", err)
	}

	currentVersion := Version

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latestVersion, assetName, err := getLatestReleaseInfo(ctx, out)
	if err != nil {
		return output.WrapFixable(err, "获取最新版本信息失败", "设置 HTTPS_PROXY 环境变量或使用代理后重试，例如: export HTTPS_PROXY=http://127.0.0.1:7890")
	}

	if latestVersion == currentVersion && !*forceFlag {
		fmt.Fprintf(out, "当前已是最新版本 v%s\n", latestVersion)
		return nil
	}

	fmt.Fprintf(out, "发现新版本 v%s（当前 v%s）\n", latestVersion, currentVersion)

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "openclaw-install-upgrade-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var downloadURL string
	if runtime.GOOS == "windows" {
		downloadURL = buildReleaseURL(assetName, latestVersion, true)
	} else {
		downloadURL = buildReleaseURL(assetName, latestVersion, false)
	}

	fmt.Fprintln(out, "下载中...")
	downloadedFile, err := downloadFileWithFallback(ctx, downloadURL, tmpDir, out)
	if err != nil {
		return output.WrapFixable(err, "下载失败", "1) 设置 HTTPS_PROXY 代理后重试\n2) 手动下载: https://github.com/goodtiger/openclaw-install/releases 放到临时目录")
	}

	fmt.Fprintln(out, "验证校验和...")
	if err := verifyChecksum(downloadedFile, latestVersion, tmpDir, out); err != nil {
		return output.WrapFixable(err, "校验和验证失败", "重新运行 upgrade 下载，或手动从 releases 页下载并校验: https://github.com/goodtiger/openclaw-install/releases")
	}

	fmt.Fprintln(out, "替换二进制...")
	newBinaryPath, err := extractBinary(downloadedFile, tmpDir, out)
	if err != nil {
		return output.WrapFixable(err, "提取二进制失败", "手动下载 https://github.com/goodtiger/openclaw-install/releases 并解压到当前目录")
	}

	backupPath := currentExe + ".backup"
	if *rollbackFlag {
		if err := createBackup(currentExe, backupPath, out); err != nil {
			return fmt.Errorf("创建备份失败: %w", err)
		}
	}

	if err := atomicReplaceBinary(newBinaryPath, currentExe, !*rollbackFlag, out); err != nil {
		if *rollbackFlag {
			rollbackFromBackup(backupPath, currentExe, currentVersion, out)
		}
		return output.WrapFixable(err, "替换二进制失败", "如果提示权限不足，请将安装器放在有写权限的目录，或使用 sudo 运行")
	}

	// Verify the new binary works
	if *rollbackFlag {
		fmt.Fprintln(out, "验证新版本...")
		if verifyErr := verifyNewBinary(currentExe); verifyErr != nil {
			fmt.Fprintf(errOut, "新版本验证失败: %v\n", verifyErr)
			fmt.Fprintln(out, "正在回滚到旧版本...")
			if rbErr := rollbackFromBackup(backupPath, currentExe, currentVersion, out); rbErr != nil {
				return fmt.Errorf("回滚失败: %v (原始错误: %w)", rbErr, verifyErr)
			}
			fmt.Fprintf(out, "回滚成功，已恢复 v%s\n", currentVersion)
			return fmt.Errorf("升级已回滚，新版本验证失败: %w", verifyErr)
		}
		fmt.Fprintln(out, "新版本验证通过！")
	}

	fmt.Fprintln(out, "升级成功！")

	return nil
}

var githubAPIURLs = []string{
	"https://api.github.com",
	"https://ghproxy.com/https://api.github.com",
}

func getLatestReleaseInfo(ctx context.Context, out io.Writer) (string, string, error) {
	var lastErr error
	for _, baseURL := range githubAPIURLs {
		apiURL := baseURL + "/repos/goodtiger/openclaw-install/releases/latest"
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("创建请求失败: %w", err)
			continue
		}

		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("User-Agent", "openclaw-install-upgrade")

		client := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(out, "尝试 %s 失败: %v\n", baseURL, err)
			lastErr = fmt.Errorf("请求 GitHub API 失败: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("GitHub API 返回错误状态: %s", string(body))
			continue
		}

		var release struct {
			TagName string `json:"tag_name"`
			Assets  []struct {
				Name string `json:"name"`
				ID   int    `json:"id"`
			} `json:"assets"`
		}

		if err := decodeJSON(resp.Body, &release); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("解析响应失败: %w", err)
			continue
		}
		resp.Body.Close()

		fmt.Fprintln(out, "检查最新版本...")

		version := strings.TrimPrefix(release.TagName, "v")

		suffix := getOSArchSuffix()
		if suffix == "" {
			return "", "", output.NewFixablef("未找到适用于 %s/%s 的构建", "前往 https://github.com/goodtiger/openclaw-install/releases 查看支持的平台，或自行编译", runtime.GOOS, runtime.GOARCH)
		}

		assetName := ""
		for _, a := range release.Assets {
			if strings.Contains(a.Name, suffix) {
				assetName = a.Name
				break
			}
		}

		if assetName == "" {
			return "", "", output.NewFixablef("未找到适用于 %s/%s 的 Release 文件", "前往 https://github.com/goodtiger/openclaw-install/releases 手动下载对应平台的压缩包", runtime.GOOS, runtime.GOARCH)
		}

		return version, assetName, nil
	}

	return "", "", fmt.Errorf("所有 GitHub 镜像均失败: %w\n\n💡 设置 HTTPS_PROXY 后重试: export HTTPS_PROXY=http://127.0.0.1:7890\n💡 手动下载: https://github.com/goodtiger/openclaw-install/releases", lastErr)
}

func getOSArchSuffix() string {
	osName := runtime.GOOS
	goarch := runtime.GOARCH

	osArchs, ok := osArchMap[osName]
	if !ok {
		return ""
	}

	suffix, ok := osArchs[goarch]
	if !ok {
		return ""
	}

	return suffix
}

func buildReleaseURL(assetName, version string, isWindows bool) string {
	baseName := assetName
	if isWindows && !strings.HasSuffix(baseName, ".zip") {
		baseName += ".zip"
	} else if !isWindows && !strings.HasSuffix(baseName, ".tar.gz") {
		baseName += ".tar.gz"
	}
	return fmt.Sprintf("/goodtiger/openclaw-install/releases/download/v%s/%s", version, baseName)
}

func downloadFileWithFallback(ctx context.Context, downloadPath string, dir string, out io.Writer) (string, error) {
	var lastErr error
	for _, baseURL := range githubDownloadURLs {
		url := baseURL + downloadPath
		result, err := downloadFile(ctx, url, dir, out)
		if err == nil {
			return result, nil
		}
		fmt.Fprintf(out, "尝试 %s 失败: %v\n", baseURL, err)
		lastErr = err
	}
	return "", fmt.Errorf("所有下载镜像均失败: %w\n\n💡 设置代理: export HTTPS_PROXY=http://127.0.0.1:7890\n💡 手动下载: https://github.com/goodtiger/openclaw-install/releases", lastErr)
}

func downloadFile(ctx context.Context, url string, dir string, out io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建下载请求失败: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("下载失败: %w", err)
	}
	tmpFile.Close()

	fmt.Fprintf(out, "已下载 %d 字节\n", written)

	return tmpPath, nil
}

func verifyChecksum(downloadedFile string, version string, dir string, out io.Writer) error {
	var lastErr error
	for _, baseURL := range githubDownloadURLs {
		checksumURL := baseURL + fmt.Sprintf("/goodtiger/openclaw-install/releases/download/v%s/SHA256SUMS", version)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("创建校验和请求失败: %w", err)
			continue
		}

		client := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			fmt.Fprintf(out, "尝试校验和镜像 %s 失败: %v\n", baseURL, err)
			lastErr = fmt.Errorf("下载校验和文件失败: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("下载校验和文件失败，状态码: %d", resp.StatusCode)
			continue
		}

		checksumData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("读取校验和文件失败: %w", err)
			continue
		}

		localFile, err := os.Open(downloadedFile)
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}

		hasher := sha256.New()
		if _, err := io.Copy(hasher, localFile); err != nil {
			localFile.Close()
			return fmt.Errorf("计算哈希失败: %w", err)
		}
		localFile.Close()
		localHash := hex.EncodeToString(hasher.Sum(nil))

		checksumFilename := filepath.Base(downloadedFile)
		expectedHash := ""
		for _, line := range strings.Split(string(checksumData), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) >= 2 {
				filename := strings.TrimPrefix(parts[1], "*")
				if filename == checksumFilename {
					expectedHash = parts[0]
					break
				}
			}
		}

		if expectedHash == "" {
			return fmt.Errorf("在校验和文件中未找到 %s", checksumFilename)
		}

		if strings.EqualFold(localHash, expectedHash) {
			fmt.Fprintf(out, "校验和验证通过: %s\n", localHash)
			return nil
		}

		return fmt.Errorf("校验和不匹配\n期望: %s\n实际: %s", expectedHash, localHash)
	}

	return fmt.Errorf("所有校验和下载镜像均失败: %w", lastErr)
}

func extractBinary(downloadedFile string, dir string, out io.Writer) (string, error) {
	if strings.HasSuffix(downloadedFile, ".zip") {
		return extractZip(downloadedFile, dir, out)
	}
	return extractTarGz(downloadedFile, dir, out)
}

func extractZip(file string, dir string, out io.Writer) (string, error) {
	r, err := zip.OpenReader(file)
	if err != nil {
		return "", fmt.Errorf("打开 ZIP 文件失败: %w", err)
	}
	defer r.Close()

	var binaryPath string
	for _, f := range r.File {
		if strings.Contains(f.Name, "openclaw-install") && !f.FileInfo().IsDir() {
			if (runtime.GOOS == "windows" && !strings.HasSuffix(f.Name, ".exe")) ||
				(runtime.GOOS != "windows" && strings.HasSuffix(f.Name, ".exe")) {
				continue
			}

			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("打开 ZIP 中的文件失败: %w", err)
			}

			tmpFile, err := os.CreateTemp(dir, "binary-*")
			if err != nil {
				rc.Close()
				return "", fmt.Errorf("创建临时文件失败: %w", err)
			}

			_, err = io.Copy(tmpFile, rc)
			rc.Close()
			tmpFile.Close()

			if err != nil {
				os.Remove(tmpFile.Name())
				return "", fmt.Errorf("提取文件失败: %w", err)
			}

			if runtime.GOOS != "windows" {
				if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
					os.Remove(tmpFile.Name())
					return "", fmt.Errorf("设置权限失败: %w", err)
				}
			}

			binaryPath = tmpFile.Name()
			fmt.Fprintf(out, "提取 %s\n", f.Name)
			break
		}
	}

	if binaryPath == "" {
		return "", errors.New("未在 ZIP 中找到 openclaw-install 二进制文件")
	}

	return binaryPath, nil
}

func extractTarGz(file string, dir string, out io.Writer) (string, error) {
	gzFile, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("打开 tar.gz 文件失败: %w", err)
	}
	defer gzFile.Close()

	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return "", fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	var binaryPath string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("读取 tar 包失败: %w", err)
		}

		baseName := filepath.Base(header.Name)
		if strings.Contains(baseName, "openclaw-install") && header.Typeflag == tar.TypeReg {
			if (runtime.GOOS == "windows" && !strings.HasSuffix(baseName, ".exe")) ||
				(runtime.GOOS != "windows" && strings.HasSuffix(baseName, ".exe")) {
				continue
			}

			tmpFile, err := os.CreateTemp(dir, "binary-*")
			if err != nil {
				return "", fmt.Errorf("创建临时文件失败: %w", err)
			}

			_, err = io.Copy(tmpFile, tarReader)
			tmpFile.Close()

			if err != nil {
				os.Remove(tmpFile.Name())
				return "", fmt.Errorf("提取文件失败: %w", err)
			}

			if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
				os.Remove(tmpFile.Name())
				return "", fmt.Errorf("设置权限失败: %w", err)
			}

			binaryPath = tmpFile.Name()
			fmt.Fprintf(out, "提取 %s\n", header.Name)
			break
		}
	}

	if binaryPath == "" {
		return "", errors.New("未在 tar.gz 中找到 openclaw-install 二进制文件")
	}

	return binaryPath, nil
}

func atomicReplaceBinary(newBinary string, currentExe string, keepBackup bool, out io.Writer) error {
	newDir := filepath.Dir(newBinary)
	currentDir := filepath.Dir(currentExe)

	if runtime.GOOS == "windows" {
		newDrive := filepath.VolumeName(newDir)
		currentDrive := filepath.VolumeName(currentDir)
		if !strings.EqualFold(newDrive, currentDrive) {
			tempFile := filepath.Join(currentDir, "."+filepath.Base(currentExe)+".tmp")
			if err := copyFile(newBinary, tempFile); err != nil {
				return fmt.Errorf("复制文件失败: %w", err)
			}
			if err := os.Chmod(tempFile, 0755); err != nil {
				os.Remove(tempFile)
				return fmt.Errorf("设置权限失败: %w", err)
			}
			if err := os.Rename(tempFile, currentExe); err != nil {
				os.Remove(tempFile)
				return fmt.Errorf("替换失败: %w", err)
			}
			return nil
		}
	} else if newDir != currentDir {
		tempFile := filepath.Join(currentDir, "."+filepath.Base(currentExe)+".tmp")
		if err := copyFile(newBinary, tempFile); err != nil {
			return fmt.Errorf("复制文件失败: %w", err)
		}
		if err := os.Chmod(tempFile, 0755); err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("设置权限失败: %w", err)
		}
		if err := os.Rename(tempFile, currentExe); err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("替换失败: %w", err)
		}
		return nil
	}

	backupPath := currentExe + ".backup"
	if err := os.Rename(currentExe, backupPath); err != nil {
		return fmt.Errorf("创建备份失败: %w", err)
	}

	if err := os.Rename(newBinary, currentExe); err != nil {
		os.Rename(backupPath, currentExe)
		return fmt.Errorf("替换失败: %w", err)
	}

	if !keepBackup {
		os.Remove(backupPath)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func createBackup(currentExe, backupPath string, out io.Writer) error {
	if _, err := os.Stat(currentExe); err != nil {
		return fmt.Errorf("当前二进制不存在: %w", err)
	}

	info, err := os.Stat(currentExe)
	if err != nil {
		return fmt.Errorf("获取当前二进制信息失败: %w", err)
	}

	freeSpace, err := getFreeSpace(filepath.Dir(backupPath))
	if err == nil && freeSpace < uint64(info.Size()*2) {
		return fmt.Errorf("磁盘空间不足，备份需要至少 %d 字节", info.Size())
	}

	if err := copyFile(currentExe, backupPath); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(backupPath, 0755); err != nil {
			os.Remove(backupPath)
			return fmt.Errorf("设置备份权限失败: %w", err)
		}
	}

	fmt.Fprintf(out, "已创建备份: %s\n", backupPath)
	return nil
}

func verifyNewBinary(binaryPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 version 命令失败: %w (输出: %s)", err, string(output))
	}
	return nil
}

func rollbackFromBackup(backupPath, currentExe, previousVersion string, out io.Writer) error {
	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("备份文件不存在: %s", backupPath)
		}
		return fmt.Errorf("备份文件无法访问: %w", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("获取备份信息失败: %w", err)
	}

	if info.Size() == 0 {
		fmt.Fprintf(out, "警告: 备份文件大小为 0，可能已损坏\n")
	}

	if runtime.GOOS == "windows" {
		if err := os.Rename(backupPath, currentExe); err != nil {
			return fmt.Errorf("恢复备份失败: %w", err)
		}
	} else {
		tempPath := currentExe + ".rollback-tmp"
		if err := os.Rename(currentExe, tempPath); err != nil {
			return fmt.Errorf("临时移动当前二进制失败: %w", err)
		}
		if err := os.Rename(backupPath, currentExe); err != nil {
			os.Rename(tempPath, currentExe)
			return fmt.Errorf("恢复备份失败: %w", err)
		}
		os.Remove(tempPath)
	}

	if err := os.Chmod(currentExe, 0755); err != nil {
		return fmt.Errorf("设置恢复后权限失败: %w", err)
	}

	return nil
}

func runRollback(args []string, out, errOut io.Writer) error {
	fs := newFlagSet("rollback", errOut, "从备份恢复上一个版本。")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前二进制路径失败: %w", err)
	}

	backupPath := currentExe + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("没有可用的备份文件 (%s)", backupPath)
		}
		return fmt.Errorf("备份文件无法访问: %w", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("获取备份信息失败: %w", err)
	}

	if info.Size() == 0 {
		fmt.Fprintf(errOut, "警告: 备份文件大小为 0，可能已损坏，是否继续恢复？(y/N) ")
		fmt.Fprint(errOut, "为安全起见，请手动删除损坏的备份文件后重试\n")
		return fmt.Errorf("备份文件损坏")
	}

	fmt.Fprintln(out, "正在从备份恢复...")
	if err := rollbackFromBackup(backupPath, currentExe, "", out); err != nil {
		return err
	}

	fmt.Fprintln(out, "回滚成功！")
	return nil
}

func decodeJSON(r io.Reader, v any) error {
	decoder := json.NewDecoder(r)
	return decoder.Decode(v)
}
