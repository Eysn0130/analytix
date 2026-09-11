# analytix-computer-use

**Analytix Computer Use** MCP server 的跨平台 npm 分发包。

此包内置以下平台的 native runtime，并由 Node launcher 按当前 `process.platform` / `process.arch` 自动选择:

- `darwin-arm64`
- `darwin-x64`
- `linux-arm64`
- `linux-x64`
- `win32-arm64`
- `win32-x64`

全局命令别名:

- `analytix-computer-use`
- `analytix-computer-use-mcp`
- `open-computer-use`
- `open-computer-use-mcp`

## 安装

```bash
npm install -g analytix-computer-use
```

根 launcher 会识别当前 `process.platform` / `process.arch`，并运行匹配的内置 native runtime。

## MCP config

如果你的 MCP client 支持 stdio 风格的 `mcpServers` JSON 配置，默认配置如下:

```json
{
  "mcpServers": {
    "analytix-computer-use": {
      "command": "analytix-computer-use",
      "args": ["mcp"]
    }
  }
}
```

包页面: https://www.npmjs.com/package/analytix-computer-use

## 使用

```bash
analytix-computer-use --version
analytix-computer-use --help
analytix-computer-use mcp
analytix-computer-use call list_apps

# macOS 权限检查与授权引导
analytix-computer-use doctor

# MCP-capable CLI 安装辅助命令
analytix-computer-use install-claude-mcp
analytix-computer-use install-gemini-mcp
analytix-computer-use install-gemini-mcp --scope user
analytix-computer-use install-opencode-mcp
```

## 说明

- 版本: `0.2.3`
- 支持的 npm 平台: `darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `win32-arm64`, `win32-x64`
- macOS 仍需要 `Accessibility` 和 `Screen Recording` 权限。
- Linux 需要已登录的桌面会话，并可使用 AT-SPI2 / D-Bus accessibility。
- Windows 需要已登录的交互式桌面会话，以便访问 UI Automation。

源码仓库: https://analytix.top
