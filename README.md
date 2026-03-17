# ssh-connect

**与任何项目无关**，Mac 本机用的 SSH 自动连接小工具。装好后在任意终端都能直接执行。

---

## 一次性安装（拷到本机任意位置）

1. 把整个 `ssh-connect` 文件夹拷到你的 Mac 上，例如：

   ```bash
   cp -r /path/to/ssh-connect ~/tools/ssh-connect
   # 或你喜欢的位置，如 ~/bin/ssh-connect
   ```

2. 编译并放到 PATH：

   ```bash
   cd ~/tools/ssh-connect
   go build -o ssh-connect .
   sudo mv ssh-connect /usr/local/bin/
   # 或者: mkdir -p ~/bin && mv ssh-connect ~/bin/  并保证 ~/bin 在 PATH 里
   ```

3. 配置（只需一次）：

   ```bash
   cp ~/tools/ssh-connect/config.yaml ~/.ssh-connect.yaml
   vim ~/.ssh-connect.yaml   # 改成你的账号、密码、IP
   ```

之后**打开任意终端**都可以直接执行 `ssh-connect`、`ssh-connect dev` 等，与当前在哪个目录、是否在某个项目里无关。

---

## 配置文件 `~/.ssh-connect.yaml`

```yaml
default:
  account: root
  password: "你的密码"
  ip: 192.168.1.100
  port: 22

servers:
  dev:
    account: root
    password: "dev密码"
    ip: 192.168.1.101
    port: 22
  prod:
    account: root
    password: "prod密码"
    ip: 10.0.0.10
    port: 22
```

- 默认读取 `~/.ssh-connect.yaml`
- 可用 `-config 文件路径` 或环境变量 `SSH_CONNECT_CONFIG` 指定配置文件

---

## 日常使用

```bash
ssh-connect              # 连 default
ssh-connect dev          # 连 servers.dev
ssh-connect prod         # 连 servers.prod
ssh-connect list         # 列出所有已配置的服务器名称
ssh-connect -config /path/to/config.yaml dev   # 指定配置文件并连接 dev
```

## 可选：alias 快速登录

在 `~/.zshrc` 里加：

```bash
alias s='ssh-connect'
alias sdev='ssh-connect dev'
alias sprod='ssh-connect prod'
```

之后 `s`、`sdev`、`sprod` 随时可用。
