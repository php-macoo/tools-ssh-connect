package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Account            string `yaml:"account"`
	Password           string `yaml:"password"`
	IP                 string `yaml:"ip"`
	Port               int    `yaml:"port"`
	Desc               string `yaml:"desc"`                // 登录时醒目提示
	IdentityFile       string `yaml:"identity_file"`       // 私钥路径，如 ~/.ssh/id_rsa
	IdentityPassphrase string `yaml:"identity_passphrase"` // 私钥密码（可选）
}

func setDefaults(s *ServerConfig) {
	if s.Account == "" {
		s.Account = "root"
	}
	if s.Port == 0 {
		s.Port = 22
	}
}

type Config struct {
	Default ServerConfig            `yaml:"default"`
	Servers map[string]ServerConfig `yaml:"servers"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	setDefaults(&cfg.Default)
	for k := range cfg.Servers {
		s := cfg.Servers[k]
		setDefaults(&s)
		cfg.Servers[k] = s
	}
	return &cfg, nil
}

// 配置固定为项目目录下的 config.yaml（二进制装到 /usr/local/bin 等位置时也读这里）
const defaultConfigPath = "/Users/macoo/tools/ssh-connect/config.yaml"

// ANSI：粗体 + 红色字体，醒目提示（无背景）
const ansiBoldRed = "\033[1;91m"
const ansiReset = "\033[0m"

func printDesc(desc string) {
	desc = fmt.Sprintf("您正在登录%s!", desc)
	content := "  " + desc + "  "
	// 按显示列数：中文/═ 多为 2 列；numEq 取大一些，保证在 ═ 占 1 列的终端里上下线也不短于文字
	contentCols := 2 + 2*utf8.RuneCountInString(desc) + 2
	numEq := contentCols + 8
	if numEq < 16 {
		numEq = 16
	}
	line := strings.Repeat("═", numEq)
	_, _ = io.WriteString(os.Stderr, ansiBoldRed+line+ansiReset+"\n")
	_, _ = io.WriteString(os.Stderr, ansiBoldRed+content+ansiReset+"\n")
	_, _ = io.WriteString(os.Stderr, ansiBoldRed+line+ansiReset+"\n")
}

// loadIdentitySigners 从 identity_file 加载私钥，返回可用于 Auth 的 signers。
func loadIdentitySigners(identityFile, passphrase string) ([]ssh.Signer, error) {
	if identityFile == "" {
		return nil, nil
	}
	path := identityFile
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取私钥 %s 失败: %w", identityFile, err)
	}
	var signer ssh.Signer
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(keyBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}
	return []ssh.Signer{signer}, nil
}

// tryDefaultIdentityFiles 尝试加载 ~/.ssh/id_ed25519 和 ~/.ssh/id_rsa，忽略错误。
func tryDefaultIdentityFiles() []ssh.Signer {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	var signers []ssh.Signer
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		path := filepath.Join(home, ".ssh", name)
		s, err := loadIdentitySigners(path, "")
		if err != nil || len(s) == 0 {
			continue
		}
		signers = append(signers, s...)
	}
	return signers
}

func buildAuthMethods(srv ServerConfig) ([]ssh.AuthMethod, error) {
	var auth []ssh.AuthMethod
	if srv.IdentityFile != "" {
		signers, err := loadIdentitySigners(srv.IdentityFile, srv.IdentityPassphrase)
		if err != nil {
			return nil, err
		}
		if len(signers) > 0 {
			auth = append(auth, ssh.PublicKeys(signers...))
		}
	} else {
		// 未配置时自动尝试默认私钥，便于仅支持公钥的服务器
		if defaultSigners := tryDefaultIdentityFiles(); len(defaultSigners) > 0 {
			auth = append(auth, ssh.PublicKeys(defaultSigners...))
		}
	}
	password := srv.Password
	if password != "" {
		keyboardInteractive := ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			if len(questions) == 0 {
				return nil, nil
			}
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = password
			}
			return answers, nil
		})
		auth = append(auth, keyboardInteractive, ssh.Password(password))
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("未配置 password 或有效的 identity_file")
	}
	return auth, nil
}

func getConfigPath() string {
	return defaultConfigPath
}

func runList(configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("配置文件不存在: %s", configPath)
		}
		return fmt.Errorf("读取配置失败: %w", err)
	}
	fmt.Println("default (默认)")
	for n := range cfg.Servers {
		fmt.Println(n)
	}
	return nil
}

func run() error {
	var name string
	flag.StringVar(&name, "name", "", "服务器名称，不填则用 default")
	flag.Parse()

	path := getConfigPath()
	if flag.NArg() > 0 && flag.Arg(0) == "list" {
		return runList(path)
	}
	if name == "" && flag.NArg() > 0 {
		name = flag.Arg(0)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("配置文件不存在: %s", path)
		}
		return fmt.Errorf("读取配置失败: %w", err)
	}

	var srv ServerConfig
	if name != "" {
		if s, ok := cfg.Servers[name]; ok {
			srv = s
		} else {
			return fmt.Errorf("未找到服务器配置: %s（可用 ssh-connect list 查看）", name)
		}
	} else {
		srv = cfg.Default
	}

	if srv.IP == "" {
		return fmt.Errorf("配置中 IP 为空，请编辑 %s", path)
	}
	if srv.Password == "" && srv.IdentityFile == "" {
		return fmt.Errorf("配置中 password 与 identity_file 至少填一项，请编辑 %s", path)
	}

	// 登录前醒目输出 desc 提示（仅输出到 stderr，不影响 SSH 会话）
	if srv.Desc != "" {
		printDesc(srv.Desc)
	}

	addr := fmt.Sprintf("%s:%d", srv.IP, srv.Port)
	auth, err := buildAuthMethods(srv)
	if err != nil {
		return err
	}
	clientConfig := &ssh.ClientConfig{
		User:            srv.Account,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		msg := fmt.Sprintf("连接 %s 失败: %v", addr, err)
		if strings.Contains(err.Error(), "unable to authenticate") {
			msg += "\n建议：1) 核对 config 中 account、password 是否正确；2) 若服务器仅支持公钥，请将本机公钥加入服务器 authorized_keys，并确保本机存在 ~/.ssh/id_rsa 或 ~/.ssh/id_ed25519（或配置 identity_file）"
		}
		return fmt.Errorf("%s", msg)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("当前不是终端，无法进入交互")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, oldState)

	w, h, _ := term.GetSize(fd)
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", h, w, modes); err != nil {
		return err
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			w, h, _ := term.GetSize(fd)
			_ = session.WindowChange(h, w)
		}
	}()

	if err := session.Shell(); err != nil {
		return err
	}

	_ = session.Wait()
	return nil
}

func main() {
	if err := run(); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}
