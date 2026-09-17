# Office 引擎本机私用安装准入材料

Status: Reference。研究日期：2026-09-15；记录落笔基线为
`5523e5bbdd68a80af09f4fb1032c1442db7c5dd6`，其他实施主体正在修改工作区。
范围仅为固定 Office 引擎资源的来源、许可材料和本机安装工程缺口。
当前事实来源为 [asset manifest](../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json)、
[asset loader](../../src/main/office/office-assets.ts) 及下述实际核对。
[既有 provenance](upstreams/code-reuse-provenance.md) 保留其源实验结论。

## 授权、目标与当前状态

用户当前任务及附件已明确授权合法的本机私用封包、安装和验收；无需再次索取
相同授权。此授权不替代第三方许可，也没有授权公开发布或向第三方分发引擎。

- `as-built`：manifest 保持 `executionMode: source-experiment`、
  `publishable: false`；独立本机私用准入已实现，`8365b16ce` 的 DMG 及隔离
  安装签名检查通过，普通 packaged 拒绝和公开门禁保持。
- `target`：完成本机私用安装后的完整产品旅程。
- `gap`：首次安装启动在应用初始化前等待 Chromium 默认 Keychain 授权，
  尚无 installed GUI 或完整产品验收。精确构建与启动证据见
  [产品能力矩阵](product-completion.md#private-installation-probe-2026-09-15)；
  本记录不构成完整许可证明或 Product RC。

## 固定资源与来源

LibreOffice/ZetaOffice buildId：
`efaf0670b4d055f838a2849becb10f08aa06a257`。
本地 `build-source-commit.json` 指向该提交，tree 为
`a7d952601fe63fd6ac31872b0fdb59d0b5c2b11e`；data 内的
`/instdir/program/versionrc` 记录同一 buildId、`ZetaOffice_24_en-US`
及 allotropia vendor。这是来源对应证据，不是完整可重现构建证明。

六项资源的准确路径、字节数和 SHA-256 统一引用 canonical manifest，避免维护
第二份字节表。本轮对六项本机文件重新计算 SHA-256，全部匹配。

| 资源 | 来源入口 | 本机已有材料及剩余缺口 |
| --- | --- | --- |
| `soffice.js` | [CDN](https://cdn.zetaoffice.net/zetaoffice_latest/soffice.js)；上述 build | 文件 hash 和 build manifest；缺生成此文件的精确 Emscripten commit、参数和适用运行时 notice 对应关系 |
| `soffice.wasm` | [CDN](https://cdn.zetaoffice.net/zetaoffice_latest/soffice.wasm)；上述 build | 解压文件及压缩文件 hash；缺 Qt/qtbase/Emscripten 精确 pins、模块和链接/构建清单 |
| `soffice.data` | [CDN](https://cdn.zetaoffice.net/zetaoffice_latest/soffice.data)；上述 build | 文件 hash、versionrc、137 个字体条目；缺完整版本对应的依赖/字体许可汇编 |
| `soffice.data.js.metadata` | [CDN](https://cdn.zetaoffice.net/zetaoffice_latest/soffice.data.js.metadata)；上述 build | 文件 hash 与 data 路径/偏移/总大小；未见独立许可声明，应随引擎生成物来源和 notice 一并记录 |
| `zeta.js` | zetajs 1.2.0，commit `57360bcb0e7726ffa0e66567c8041261b959f8dd`；[固定文件](https://github.com/allotropia/zetajs/blob/57360bcb0e7726ffa0e66567c8041261b959f8dd/source/zeta.js) | 原始文件与 MIT LICENSE；安装资源需要实际保留许可证，已有适配代码 notice 不代替安装产物检查 |
| `NotoSansCJKsc-Regular.otf` | Version 2.004，commit `f8d157532fbfaeda587e826d4cd5b21a49186f7c`；[固定文件](https://github.com/notofonts/noto-cjk/blob/f8d157532fbfaeda587e826d4cd5b21a49186f7c/Sans/OTF/SimplifiedChinese/NotoSansCJKsc-Regular.otf) | 未修改字体、`font-assets.json`、`FONT-NOTICE.txt`、OFL；需一起保留于安装资源 |

CDN URL 中的 `latest` 可变，不是固定版本地址。本轮仅对 CDN 作 HEAD 检查，
四项均为 HTTP 200；文件大小与本地原始/压缩记录相符。未重新下载大型引擎，
HEAD 不证明远端正文 hash。身份判断以本机重新核对的 manifest hash 为准。

## 可复核的许可证文本

下表前五项本地文本已与固定上游原文计算 SHA-256 比对一致。Qt、Emscripten
两项只核对了官方声明所指分支当时返回的许可证，未将它们当作精确构建绑定。

| 材料 | 官方来源 | SHA-256 |
| --- | --- | --- |
| LO `COPYING`，GPLv3 | [固定提交](https://git.libreoffice.org/core/+/efaf0670b4d055f838a2849becb10f08aa06a257/COPYING) | `8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903` |
| LO `COPYING.LGPL`，LGPLv3 | [固定提交](https://git.libreoffice.org/core/+/efaf0670b4d055f838a2849becb10f08aa06a257/COPYING.LGPL) | `a853c2ffec17057872340eee242ae4d96cbf2b520ae27d903e1b2fef1a5f9d1c` |
| LO `COPYING.MPL`，MPL 2.0 | [固定提交](https://git.libreoffice.org/core/+/efaf0670b4d055f838a2849becb10f08aa06a257/COPYING.MPL) | `1f256ecad192880510e84ad60474eab7589218784b9a50bc7ceee34c2b91f1d5` |
| zetajs MIT | [固定 LICENSE](https://raw.githubusercontent.com/allotropia/zetajs/57360bcb0e7726ffa0e66567c8041261b959f8dd/LICENSE) | `a8c9b7d037ed112c3b9f85d5a1122aedd218947b21cf3f2c4200d13d7fe1a7d9` |
| Noto OFL 1.1 | [固定 Sans/LICENSE](https://raw.githubusercontent.com/notofonts/noto-cjk/f8d157532fbfaeda587e826d4cd5b21a49186f7c/Sans/LICENSE) | `6a73f9541c2de74158c0e7cf6b0a58ef774f5a780bf191f2d7ec9cc53efe2bf2` |
| qtbase `LICENSE.LGPL3` | [5.15.2+wasm 分支](https://raw.githubusercontent.com/allotropia/qtbase/5.15.2%2Bwasm/LICENSE.LGPL3) | `da7eabb7bafdf7d3ae5e9f223aa5bdc1eece45ac569dc21b3b037520b4464768` |
| Emscripten `LICENSE` | [fixed-3.1.65 分支](https://raw.githubusercontent.com/allotropia/emscripten/fixed-3.1.65/LICENSE) | `620a78084fc7ca97c0b5dea9abf891f3ffcadfdbf305276f099c9c4e12fc1d86` |

zetajs notice 为 Copyright (c) 2024 allotropia software GmbH and contributors；
Noto 字体 notice 为 © 2014–2021 Adobe。它们的原始许可及版权需随资源保留。
Emscripten LICENSE 除 MIT/NCSA 外还说明 Node、musl 等材料；不能仅复制其
首段许可名称就称已覆盖实际嵌入的运行时。

## 137 个内置字体：元数据观察与待补材料

读取范围为现有 data metadata 指定字体的 SFNT name 表，不复制字体或引擎。
137 是文件条目数，包含两个路径下的 OpenSymbol，不是独立字体家族数。
metadata 中只有 license dialog UI 路径，没有完整安装 LICENSE/NOTICE 文本。

| 条目数 | 内嵌声明分类 | 待补材料 |
| --- | --- | --- |
| 73 | OFL 1.1：Amiri、Carlito、David/Miriam Libre、Rubik、Gentium、常规 Liberation、Noto、Scheherazade 等 | 按实际字体版本汇总版权、OFL、Reserved Font Names；部分已有完整内嵌文本 |
| 4 | Caladea：Apache-2.0 | 对应版本版权、完整 Apache 文本及适用 NOTICE |
| 22 | DejaVu：Bitstream/Arev 声明及公共领域修改说明；其中 Math TeX Gyre 仅有许可 URL | 保留不同子集的完整条款；补 Math 对应版本许可 |
| 4 | Liberation Sans Narrow：LiberationFontLicense URL | 固定旧版专用许可及字体例外，不能套用其他 Liberation 字体的 OFL |
| 10 | Linux Libertine/Biolinum：内嵌文本写 `GPL ... AND OFL` | 核对原始发行许可选择说明及完整文本；不自行将 AND 改写为 OR |
| 18 | CLM：没有独立 license name 字段，但版权字段声明 GPLv2 | 对应发行版权、完整 GPL 及任何字体例外 |
| 6 | 两个 OpenSymbol、两个 Alef、两个 Frank Ruhl Hofshi 条目只读到版权，缺独立许可字段 | 从固定 LO source 的字体/发行许可清单和精确字体上游补对应许可 |

上述分类是本机元数据证据，不是全部字体完成许可审查。已读声明中没有发现
专门禁止本机私用的条款；缺失字段也不证明无许可或存在收费要求。六个仅版权
条目及旧字体的精确许可仍需补材料，不能把它们归入“全部许可已通过”。
现有 Noto CJK 的 OFL 不覆盖 data 内其他字体。

后续有界核验已补到同一固定 LO 提交的
[license.xml](https://raw.githubusercontent.com/LibreOffice/core/efaf0670b4d055f838a2849becb10f08aa06a257/readlicense_oo/license/license.xml)：
588864 bytes，SHA-256
`0fc511d0cdf0b1098b2000f186840629f4a4740a0efe9a8cf60753d7de0f6af8`。
主线程已下载并核对该正文，保留于上述 host 资源目录。它明确覆盖 Alef、Frank
Ruhl Hofshi 的 OFL 1.1，旧 Liberation 专用 GPLv2 条款，以及 Libertine/Biolinum
G 的 GPLv2 加字体例外和额外 **OFL 1.0** 文本；不擅自改变许可选择关系。

选入的 18 个 CLM 字体来自 David、FrankRuehl、Miriam、MiriamMono、Nachlieli。
这些家族对应的 XML 段落没有字体例外，不能套用同节其他家族的例外。
Alef/Frank Ruhl 的四条缺字段记录因此已有明确许可正文；两个 OpenSymbol 实际
条目的 hash 均与固定 `download.lst` 的 102.12 版本相符，但其专用许可证明仍未
闭合。`OpenSymbol2.sfd` 是另一款 OpenSymbolMath，不能用其 OFL 1.1 为当前
OpenSymbol 代作证明。完整许可选择及逐字体归档字节比对仍有剩余工作。

## 官方文本支持的使用范围

[GNU FAQ](https://www.gnu.org/licenses/gpl-faq.html#GPLRequireSourcePostedPublic)
明确允许修改后私用而不公开，另在
[组织内部使用说明](https://www.gnu.org/licenses/gpl-faq.html#InternalDistribution)
区分组织自用复制与对外分发。
[Mozilla FAQ Q5–Q8](https://www.mozilla.org/en-US/MPL/2.0/FAQ/)
说明 MPL 对私用及组织内部使用的范围，并把对外分发的源码告知义务单独列出。
这些文本支持区分本机私用与对外发行，不自动证明未知组件均受这些许可覆盖。

MIT、OFL 文本授予使用权限；OFL 要求在随软件复制/捆绑字体时保留版权和
许可，并规定修改字体名称等条件。当前 Noto 为未修改字体。
[LibreOffice 官方许可说明](https://www.libreoffice.org/licenses/)
明确产品还含随版本变化的其他第三方许可，根 MPL 不能覆盖完整运行包。

[Qt 官方义务说明](https://www.qt.io/development/open-source-lgpl-obligations)
区分开源与商业路径，说明适用 LGPL 的源码、通知、重链接及安装条件，并提示
某些模块仅提供 GPL 开源选项。静态 WASM 的具体模块和链接方式需要实际构建
材料判断；不能仅因使用 Qt 就宣称必须采购，也不能据 LGPL 名称宣称可任意分发。
本轮没有发现需要采购才能本机私用的证据；这不是“全部许可均已通过”的结论。

## 工程收敛与真正的外部资料依赖

现有授权足以继续本机路径，不应以等待对外发行材料为由重复索取用户授权。
本机工程可独立完成：六项 hash 绑定、已有 notice 的归档与安装保留、字体材料
映射、受限本机准入实现及验证。font notice 缺项优先从固定 LO source 的发行
许可材料与对应公开字体版本补齐；不要无证据转成采购或上游授权阻塞。

如果要证明**这份既有二进制可对外分发且具有完整对应源码/重链接材料**，当前
资料中真正不能靠声明补齐的是：

1. 产生此 build 的 Qt/qtbase、Emscripten 精确 commits、补丁、Qt 模块与构建/
   链接选项。固定 zetajs [README](https://github.com/allotropia/zetajs/blob/57360bcb0e7726ffa0e66567c8041261b959f8dd/README.md)
   只列 LO、Qt 和 Emscripten 的分支，不能将今日分支 HEAD 倒推为历史 build。
2. 与该二进制相配套、适用分发路径要求的完整对应源码、重链接/安装材料和
   版本化依赖许可汇编。现有 LO commit 和三份根许可证不足以替代这些材料。

继续沿用此二进制进行完整对外准入时，需要上游提供匹配资料；另行固定来源并
自行重建是不同工程路线。本机私用不应直接套用上述对外分发分母。公开发布和
对外分发门禁仍保持，且 `publishable: false` 不能被展示成“可分发”。

当前候选已实现独立本机准入，保留源实验拒绝和公开门禁。Go 从实际 executable
验证既有 packaged authority 与 macOS nonpublishable resource seal，再检查固定
`Resources/office-private` 的资格、源码快照和 35 项文件闭包。源注册的归因不会
因此变成正式发行回执。Host、原生适配器和 Main 准入请求复核当前资源身份。
Main 通过受保护的专用 local-display 请求获取一次性关联的资格摘要，再以 held
读取核对原生引擎、surface 和 preload；不接受 Renderer 提交的准入标志或路径。

专用入口 `scripts/package-office-private-local.mjs` 只允许隔离 ARM64 本机候选，
新建输出目录并固定 `--publish never`。afterPack 在既有 authority/staged payload
与签名封印前复制完整资源和 notices。普通构建拒绝未授权的私有 payload，正式
发行意图不能采用该路径。生成 codec 另行打成仅依赖 Node builtins 的独立目录，
精确解包，并由实际文件集合纳入原有包权威。上述代码仍需真实安装验收；不以
候选实现或单元测试宣称正常功能、回滚或原生渲染已通过。后续已取得精确
`8365b16ce` 的实际 DMG 与安装签名证据，首次启动的系统授权缺口仍未闭合。

## Host-scope 证据位置

仅用于此次配置主机复核，不是跨主机路径契约：

`/Volumes/AnalytixCache/development-v3/tmp/unified-workbench-20260914/office-experiments/zetajs`

读取的材料包括根部 `build-manifest.json`、`build-source-commit.json`、
`source-tree.json`、三份 COPYING、六项 manifest 资源、data metadata 指定的字体
name 表，以及 `fonts/font-assets.json`、`fonts/FONT-NOTICE.txt`、`fonts/OFL.txt`
和固定 zetajs 目录的 LICENSE/来源说明。未读取用户文档、历史、profile 或凭据。

## 本次证据边界

完成：本机六资源 hash、上述固定许可原文 hash、字体元数据分类、官方当前文本
范围核对及独立本机准入候选代码。未完成：完整对外发行构建对应关系、全部
内置字体精确许可闭合、完整安装旅程与 packaged GUI 验收。实现及精确安装
签名验证记录见产品能力矩阵；
许可材料不替代运行验收，源码与合成数据检查也不替代完整产品验收。
