#!/usr/bin/env node
const mcpConfig = {
  "mcpServers": {
    "analytix-computer-use": {
      "command": "analytix-computer-use",
      "args": [
        "mcp"
      ]
    }
  }
};
const lines = [
  "",
  "已安装 analytix-computer-use@0.2.3。",
  "包地址: https://www.npmjs.com/package/analytix-computer-use",
  "命令: analytix-computer-use, analytix-computer-use-mcp, open-computer-use, open-computer-use-mcp",
  "将从内置 artifacts 中选择 " + process.platform + "-" + process.arch + " 对应的 native runtime。",
  "",
  "下一步:",
  "1. 运行 analytix-computer-use --version",
  "2. 将下面的 MCP config 添加到你的 host client",
  "3. macOS 上运行 analytix-computer-use doctor，并按提示授予辅助功能 / 屏幕录制权限",
  "",
  "MCP config:",
  JSON.stringify(mcpConfig, null, 2),
  "",
];
for (const line of lines) {
  console.log(line);
}
