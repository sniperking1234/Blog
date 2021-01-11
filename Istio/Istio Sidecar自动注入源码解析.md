## Istio Sidecar 介绍

在Sidecar部署方式中会为每个应用的容器部署一个伴生容器。对于 Istio，Sidecar 接管进出应用程序容器的所有网络流量。

在 Kubernetes 的 Pod 中，在原有的应用容器旁边运行一个 Sidecar 容器，可以理解为两个容器共享存储、网络等资源，可以广义的将这个注入了 Sidecar 容器的 Pod 理解为一台主机，两个容器共享主机资源。Istio 的 Sidecar 容器时自动注入的，无需人工干预。

## Istio Sidecar 注入原理

Istio SideCar 注入通过 Kubernetes 的准入控制器 Admission Controller 实现。准入控制器会拦截 Kubernetes API Server 收到的请求，拦截发生在认证和鉴权完成之后，对象进行持久化之前。可以定义两种类型的 Admission webhook：Validating 和 Mutating。Validating 类型的 Webhook 可以根据自定义的准入策略决定是否拒绝请求；Mutating 类型的 Webhook 可以根据自定义配置来对请求进行编辑。Sidecar 注入的时候，使用的是 Mutating 注入 Sidecar 的 Yaml。

看一下 Mutating 的配置：
```yaml
webhooks:
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: 
    service:
      name: istiod
      namespace: istio-system
      path: /inject
      port: 443
  failurePolicy: Fail
  matchPolicy: Exact
  name: sidecar-injector.istio.io
  namespaceSelector:
    matchLabels:
      istio-injection: enabled
  objectSelector: {}
  reinvocationPolicy: Never
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - v1
    operations:
    - CREATE
    resources:
    - pods
    scope: '*'
  sideEffects: None
  timeoutSeconds: 30
```
namespaceSelector 用来选择符合条件的命名空间，**也就是说 Sidecar 注入的首要条件是，要在命名空间上标记 `istio-injection: enabled` 标签**。命名空间有了这个标签之后，在这个命名空间下创建 Pod 的时候，在持久化 Pod yaml之前，会到 istio-system 命名空间下 istiod 服务中发送 inject 请求，由 Istio 处理具体的注入逻辑。所以当 istiod pod 挂掉的时候，注入 sidecar 也是不成功的。
## 参考
https://www.luozhiyun.com/archives/397

