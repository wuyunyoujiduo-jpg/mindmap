## 安装（macOS）

1. 下载下面的 `.dmg`，打开后把 **MindMap** 拖进「应用程序」。
2. 应用没有经过苹果公证，第一次打开前在「终端」执行：

   ```sh
   xattr -cr /Applications/MindMap.app
   ```

3. 双击打开。Intel 和 Apple 芯片的 Mac 都可以使用。

## 安装（Windows）

下载下面的 `MindMap Setup x.y.z.exe` 安装。应用没有代码签名，Windows 可能提示「已保护你的电脑」，点「更多信息 → 仍要运行」。快捷键里的 ⌘ 在 Windows 上对应 Ctrl。

## 一键更新

装好 `gh`（`brew install gh && gh auth login`）后，用仓库里的 `update.sh` 更新，不会弹安全提示：

```sh
~/mindmap-update.sh
```

第一次获取脚本的方法见仓库 README 的「一键更新」。
