# 离线安装包优化规划

更新时间：2026-07-28

## 已实施方案

当前同时发布两种 Windows x64 NSIS 安装包：

- **标准版（推荐）**：安装包不捆绑 WebView2。Windows 已有 Evergreen Runtime 时直接使用；缺少时由安装程序调用微软官方 Bootstrapper 获取并静默安装；
- **完整离线版**：捆绑 WebView2 Evergreen Runtime，适合无外网、隔离网络或统一离线部署。

两种安装包包含完全相同的 Codex Continuity 功能，差异仅在 WebView2 的交付方式。MSI 暂不发布，待企业集中部署、组策略或软件分发平台出现明确需求后再恢复。

## WebView2 的作用

Codex Continuity 是 Tauri 桌面应用：

- React、HTML 和 CSS 构成界面；
- WebView2 使用 Microsoft Edge 渲染引擎显示这些本地界面；
- Rust 负责窗口、托盘、凭据、加密、文件和本机调用；
- Go sidecar 负责会话扫描、同步和服务端通信。

WebView2 只负责客户端界面渲染，不负责访问 Codex 或 OpenAI，服务端也不需要安装 WebView2。

## 当前体积

| 组成 | 大小 |
| --- | ---: |
| NSIS 标准安装包 | 5.91 MiB |
| NSIS 完整离线安装包 | 202.76 MiB |
| WebView2 离线安装程序 | 194.37 MiB |
| Rust 桌面程序 | 15.19 MiB |
| Go 同步核心 | 6.55 MiB |
| 便携压缩包 | 7.79 MiB |

WebView2 离线安装程序本身已经压缩并签名，因此 194.37 MiB 是完整离线单文件方案的主要体积下限。标准版把这部分改为“仅在目标电脑缺失时从微软官方获取”，大多数符合条件的 Windows 10/11 设备无需重复下载。

## 10 Mbps 带宽估算

若服务器标称的“10M”指 10 Mbps：

- 理论最高速度约 1.25 MB/s；
- 203 MiB 理论下载时间约 2 分 50 秒；
- 考虑协议、线路和磁盘，实际通常约 3–5 分钟；
- 5 人同时下载且均分带宽时，理论约 14 分钟/人；
- 10 人同时下载时，理论约 28 分钟/人。

若“10M”实际指 10 MB/s，则单包约 20–25 秒。应以服务商控制台标注的 Mbps 或 MB/s 为准。

## 构建与发布

```powershell
# 同时构建标准版、完整离线版和便携包
.\scripts\build-desktop.ps1

# 只构建某一种安装包
.\scripts\build-desktop.ps1 -PackageMode Standard
.\scripts\build-desktop.ps1 -PackageMode Offline
```

构建脚本会生成 `desktop-release.json`。服务端下载页根据该清单显示版本、真实体积、下载地址与选择说明，不再把 MSI 或固定文件名写死在界面中。

## 后续优化路线

### 方案 A：体积优先

- 继续使用 NSIS LZMA。
- 为 Rust 增加 `lto`、`strip`、`panic = "abort"` 和单 codegen unit。
- Go sidecar 继续使用现有 `-trimpath -ldflags "-s -w"`。

预估结果：约 200–203 MiB。当前已经接近该方案的体积下限，预计只能减少 0–3 MiB。

### 完整离线版安装速度优化

- WebView2 继续使用 `offlineInstaller`。
- NSIS 压缩从默认 LZMA 调整为 zlib。
- 同时应用 Rust 体积优化。

预估结果：约 203–208 MiB。体积可能增加少量，但 NSIS 自身解压 CPU 开销更低。总安装时间仍需在同一台 Windows 机器上实测，因为 WebView2 安装过程也占用时间。

### 方案 C：不压缩

- NSIS 使用 `compression: "none"`。

预估结果：约 215–225 MiB。解压最快，但下载更慢，不适合 10 Mbps 服务端。

## 后续实施顺序

1. 保留当前标准版与完整离线版作为双产品基线。
2. 在同一台 Windows 设备记录完整离线版：
   - 安装包大小；
   - 点击安装到完成的总时间；
   - “正在解压缩”阶段耗时；
   - WebView2 阶段耗时；
   - 峰值 CPU 和磁盘占用。
3. 构建 zlib 对照包。
4. 若 zlib 仅增加少量体积但明显缩短解压时间，将 zlib 设为正式完整离线版。

## 后续带宽优化

完整离线包用于首次安装。后续升级不应重复分发 194.37 MiB WebView2，可增加：

- 约 8–12 MiB 的应用升级包；
- 独立 WebView2 离线依赖包，仅在目标电脑缺少运行时的时候下载；
- HTTP Range、ETag 和 SHA-256，以支持断点续传与完整性校验；
- 公司内网 GitLab Release、共享盘或内部下载节点，避免所有同事同时占用个人服务器 10 Mbps 出口。

## 图标决定

保留现有连续链路纹样，颜色改为：

- 背景：`#3A3F47` → `#20242A` → `#090B0E` 石墨黑渐变；
- 线条：`#CFD3DA` 银灰色；
- 不使用蓝色、纯白色或彩色高光。

预览文件：[app-icon-graphite-preview.png](../design/app-icon-graphite-preview.png)
