## 运维小工具

基于 APISIX 的蓝绿部署工具：拉取新镜像，起一个随机端口的新容器，健康检查通过后把 APISIX upstream 切到新容器，旧容器保留用于回滚。

需要在当前目录准备 `nodes.yaml`（参考 `nodes.example.yaml`）和 `compose.yaml`（参考 `compose.example.yaml`）。

```bash
rolld                   # 交互模式（带补全），每条命令执行前会重新读取 nodes.yaml / compose.yaml
rolld up <srv>          # 部署 srv 的新容器并切换 upstream
rolld rollback <srv>    # 把 upstream 切回上一个容器，并删除当前容器
rolld prune <srv|all>   # 删除旧容器：保留最新的和 upstream 正在指向的
rolld help              # 查看用法
```

非交互模式执行一条命令后退出：成功退出码为 `0`，失败为 `1`，参数错误为 `2`，可以直接在脚本或 CI 里使用。
