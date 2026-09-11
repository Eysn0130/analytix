---
name: analytix-computer-use
description: 使用 Analytix Computer Use MCP server 通过截图、accessibility tree 和语义 element_index 操作本地桌面应用。
---

# Analytix Computer Use

当任务需要读取或操作本地桌面 UI 时，使用 Analytix Computer Use。

## 工作流

1. 先用 `analytix-computer-use doctor` 检查后端和权限状态。
2. 用 `analytix-computer-use call list_apps` 查看可用应用。
3. 用 `analytix-computer-use call get_app_state --args '{"app":"TextEdit"}'` 获取应用状态。
4. 优先使用返回的 accessibility tree 里的 `element_index` 执行动作。
5. 选择文本时优先用 `select_text`，不要用拖拽模拟选择，除非目标应用不支持语义文本范围。
6. 输入中文、emoji 或组合字符时仍可使用 `type_text`；macOS 后端会优先通过当前可编辑元素的 accessibility value 和选区/光标插入，避免输入法干扰。
7. 只有在没有语义元素可用时才使用坐标动作。
8. 每次动作后检查返回的截图，或再次调用 `get_app_state` 再继续。

## 常用命令

```bash
analytix-computer-use doctor
analytix-computer-use call list_apps
analytix-computer-use call get_app_state --args '{"app":"TextEdit"}'
analytix-computer-use call set_value --args '{"app":"TextEdit","element_index":"2","value":"Hello from Analytix"}'
analytix-computer-use call select_text --args '{"app":"TextEdit","element_index":"2","text":"Hello from Analytix"}'
analytix-computer-use call type_text --args '{"app":"TextEdit","text":"More text"}'
analytix-computer-use call press_key --args '{"app":"TextEdit","key":"Return"}'
```
