容器运行时插件（Container Runtime Interface，简称 CRI）是 Kubernetes v1.5 引入的容器运行时接口，它将 Kubelet 与容器运行时解耦，将原来完全面向 Pod 级别的内部接口拆分成面向 Sandbox 和 Container 的 gRPC 接口，并将镜像管理和容器管理分离到不同的服务。
![](https://user-images.githubusercontent.com/7834655/111949721-30dbfa00-8b1c-11eb-9e42-4aa5d5b830bc.png)
CRI 最早从从 1.4 版就开始设计讨论和开发，在 v1.5 中发布第一个测试版。在 v1.6 时已经有了很多外部容器运行时，如 frakti 和 cri-o 等。v1.7 中又新增了 cri-containerd 支持用 Containerd 来管理容器。
采用 CRI 后，Kubelet 的架构如下图所示：
![](https://user-images.githubusercontent.com/7834655/111950089-c5def300-8b1c-11eb-9dce-0be0e72820b3.png)