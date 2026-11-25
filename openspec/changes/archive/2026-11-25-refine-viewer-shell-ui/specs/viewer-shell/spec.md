# Spec Delta

## ADDED Requirements

### Requirement: Windows 上的自定义标题栏

应用在 Windows 上 SHALL 隐藏系统标题栏，由应用顶栏同时充当标题栏：顶栏沿用应用深色配色，保留系统的最小化、最大化与关闭按钮，并让顶栏内容避开这些按钮。

#### Scenario: 系统为浅色模式

- **WHEN** Windows 使用浅色外观时启动应用
- **THEN** 窗口顶部不出现浅色系统标题栏，顶栏保持应用的深色配色

#### Scenario: 拖动标题栏

- **WHEN** 用户在顶栏空白处按住并拖动
- **THEN** 窗口随指针移动

#### Scenario: 双击标题栏

- **WHEN** 用户双击顶栏空白处
- **THEN** 窗口在最大化与还原之间切换，与系统标题栏的双击行为一致

#### Scenario: 窗口按钮不遮挡顶栏操作

- **WHEN** 窗口缩到允许的最小宽度
- **THEN** 顶栏右侧的操作按钮完整可见，且不与系统窗口按钮重叠

#### Scenario: 非 Windows 平台

- **WHEN** 应用在 macOS 或 Linux 上运行
- **THEN** 保留系统标题栏，顶栏不额外为窗口按钮预留空间
