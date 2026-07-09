# Print Server

多打印机管理控制台 | Multi-Printer Management Console

## 项目简介

Print Server 是一个基于 Go 语言开发的多打印机管理控制台应用，支持同时管理多台网络打印机和 USB 打印机。该项目采用分层架构设计，为多品牌、多通道、云端集成提供了良好的扩展基础。

## 主要特性

- **多打印机管理**：支持同时管理多台打印机，包括网络打印机和 USB 打印机
- **多品牌支持**：兼容 Gprinter、Feie 等主流热敏打印机品牌
- **ESC/POS 指令**：支持完整的 ESC/POS 命令集，包括文字、条码、二维码、图片等
- **MQTT 集成**：支持 MQTT 协议与云端系统对接，实现远程打印和状态同步
- **Windows 桌面界面**：提供直观的 Windows GUI 界面，方便用户管理打印机
- **打印任务管理**：支持打印任务排队、重试、取消等操作
- **状态监控**：实时监控打印机在线状态和作业状态
- **文档服务**：内置 API 文档服务器

## 技术栈

- **语言**：Go
- **GUI 框架**：Walk (Windows GUI Library)
- **通信协议**：TCP/IP, USB, MQTT
- **打印协议**：ESC/POS

## 项目结构

```
├── main.go                      # 程序入口
├── go.mod / go.sum              # Go 依赖管理
├── build.ps1                    # 构建脚本
├── app.manifest                 # Windows 应用清单
│
├── internal/
│   ├── api/                     # API 处理层
│   │   ├── Print.go             # 打印请求处理
│   │   └── Print_test.go        # 测试用例
│   │
│   ├── config/                  # 配置管理
│   │   └── Config.go            # 配置文件加载与保存
│   │
│   ├── docserver/               # API 文档服务器
│   │   ├── Server.go            # 文档服务实现
│   │   └── apidoc.html          # API 文档页面
│   │
│   ├── errcode/                 # 错误码定义
│   │   └── Errors.go            # 错误类型和错误码
│   │
│   ├── escpos/                  # ESC/POS 指令集
│   │   ├── Barcode.go           # 条码指令
│   │   ├── Buzzer.go            # 蜂鸣器指令
│   │   ├── Command.go           # 基本指令构建
│   │   ├── Finish.go            # 打印完成指令
│   │   ├── Layout.go            # 布局计算
│   │   ├── QRCode.go            # 二维码指令
│   │   ├── Raster.go            # 光栅图形
│   │   ├── Receipt.go           # 小票模板
│   │   └── Reprint.go           # 重打指令
│   │
│   ├── instance/                # 单实例控制
│   │   └── SingleInstance.go    # 确保程序单实例运行
│   │
│   ├── layout/                  # 打印内容布局
│   │   ├── Element.go           # 布局元素定义
│   │   ├── Graphics.go          # 图形渲染 (二维码、条码、图片)
│   │   ├── Render.go            # 渲染引擎
│   │   ├── Sample.go            # 样例内容
│   │   ├── Table.go             # 表格渲染
│   │   └── Text.go              # 文本渲染
│   │
│   ├── logger/                  # 日志模块
│   │   └── Logger.go            # 日志记录
│   │
│   ├── model/                   # 数据模型
│   │   ├── Job.go               # 打印任务模型
│   │   ├── Printer.go           # 打印机模型
│   │   └── Settings.go          # 设置模型
│   │
│   ├── mqtt/                    # MQTT 客户端
│   │   ├── Client.go            # MQTT 连接管理
│   │   └── Report.go            # 状态上报
│   │
│   ├── printsvc/                # 打印服务
│   │   ├── Events.go            # 打印事件
│   │   ├── Service.go           # 打印任务调度
│   │   └── Status.go            # 状态查询
│   │
│   ├── transport/               # 传输层
│   │   ├── Ping.go              # ICMP Ping 检测
│   │   ├── Printer.go           # 打印机接口定义
│   │   ├── Spooler.go           # Windows 打印池
│   │   ├── SpoolerStatus.go     #  spooler 状态查询
│   │   ├── Status.go            # 打印机状态
│   │   └── Tcp.go               # TCP 连接
│   │
│   ├── ui/                      # 用户界面
│   │   ├── AddWizard.go         # 添加打印机向导
│   │   ├── AlertWindow.go       # 警报窗口
│   │   ├── App.go               # 主应用窗口
│   │   ├── Icon.go              # 应用图标
│   │   ├── JobsView.go          # 打印任务视图
│   │   ├── JsonTest.go          # JSON 测试工具
│   │   ├── Notify.go            # 通知功能
│   │   ├── OnlineDebounce.go    # 在线状态防抖
│   │   ├── PrintersView.go      # 打印机列表视图
│   │   ├── Properties.go        # 属性编辑器
│   │   ├── SettingsView.go      # 设置页面
│   │   ├── StatusMonitor.go     # 状态监控
│   │   ├── Tables.go            # 表格组件
│   │   └── Tray.go              # 系统托盘
│   │
│   ├── util/                    # 工具函数
│   │   └── Encoding.go          # 编码转换 (GB18030)
│   │
│   └── assets/                  # 静态资源
│       └── app.png             # 应用图标
```

## 功能说明

### 打印机管理

- 支持添加、编辑、删除打印机
- 支持网络打印机 (TCP/IP) 和 USB 打印机
- 支持多品牌：Gprinter、Feie 等
- 打印机状态实时监控

### 打印功能

- 支持 JSON 格式的打印内容定义
- 支持文本、表格、二维码、条码、图片等元素
- 支持自定义纸张宽度 (58mm, 80mm 等)
- 支持对齐方式、字体大小、加粗等样式

### MQTT 云端对接

- 支持 MQTT 协议连接云端服务器
- 打印机状态上报
- 接收云端打印任务

### API 文档

内置 API 文档服务器，提供完整的打印接口说明。

## 构建与运行

### 环境要求

- Go 1.18+
- Windows 10/11
- GCC (用于 CGO)

### 构建步骤

```powershell
# 使用 PowerShell 构建
./build.ps1
```

### 运行

构建完成后，运行生成的可执行文件即可启动应用。

## 配置说明

配置文件默认保存在程序同目录下，主要配置包括：

- 打印机列表配置
- MQTT 连接设置
- API 文档服务器设置

## 许可证

本项目仅供学习参考使用。