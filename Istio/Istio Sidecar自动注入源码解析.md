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

## Sidecar 注入条件判断

```go
func injectRequired(ignored []string, config *Config, podSpec *corev1.PodSpec, metadata *metav1.ObjectMeta) bool {
	if podSpec.HostNetwork {
		return false
	}
	
	for _, namespace := range ignored {
		if metadata.Namespace == namespace {
			return false
		}
	}

	annos := metadata.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}

	var useDefault bool
	var inject bool
	switch strings.ToLower(annos[annotation.SidecarInject.Name]) {
	// http://yaml.org/type/bool.html
	case "y", "yes", "true", "on":
		inject = true
	case "":
		useDefault = true
	}

	// If an annotation is not explicitly given, check the LabelSelectors, starting with NeverInject
	if useDefault {
		for _, neverSelector := range config.NeverInjectSelector {
			...
			}
		}
	}

	// If there's no annotation nor a NeverInjectSelector, check the AlwaysInject one
	if useDefault {
		for _, alwaysSelector := range config.AlwaysInjectSelector {
			...
			}
		}
	}

	var required bool
	switch config.Policy {
	default: // InjectionPolicyOff
		log.Errorf("Illegal value for autoInject:%s, must be one of [%s,%s]. Auto injection disabled!",
			config.Policy, InjectionPolicyDisabled, InjectionPolicyEnabled)
		required = false
	case InjectionPolicyDisabled:
		if useDefault {
			required = false
		} else {
			required = inject
		}
	case InjectionPolicyEnabled:
		if useDefault {
			required = true
		} else {
			required = inject
		}
	}

	return required
}
```
上面代码是 Sidecar 是否符合注入条件的判断，下面总结一下代码中的逻辑：
1. 判断 Pod 是否开启了`HostNetwork`，如果开启了 HostNetwork 这个特性，那么 Sidecar 中对网络规则的修改就会应用到宿主机的网络中，所以在这里不能注入 Sidecar。
2. 判断 Pod 所在的命名空间是否需要注入，在这里会将`kube-system`和`kube-public`命名空间下的 Pod 排除，这两个命名空间下都是 K8S 的系统组件 Pod，注入会造成整个集群不可用。
3. 判断 Pod 中是否有注解`sidecar.istio.io/inject`且值为 "y", "yes", "true", "on", 如果有，则认为开启注入。
4. 判断 Pod 的标签是否在 `istio-sidecar-injector` cm 中的 neverInjectSelector 配置当中。如果有，则不开启注入，此配置会覆盖条件 3。
5. 判断 Pod 的标签是否在 `istio-sidecar-injector` cm 中的 alwaysInjectSelector 配置当中。如果有，则开启注入，此配置会覆盖条件 3。如果已经通过了条件 4 ，则不会进入到条件 5 当中。
6. 如果没有进入过条件 3、4、5，那么此时 `useDefault` 为 `true`，这时候判断`istio-sidecar-injector` cm 中的 policy 字段，如果是 `enable`，则开启 Sidecar 注入。

## 注入 Pod
如果符合 Sidecar 注入条件，那么就开启 Pod 注入，进入到`injectPod`函数中。在这个函数中，又调用`InjectionData`方法，生成了注入的 pod yaml。
```go
func InjectionData(params InjectionParameters, typeMetadata *metav1.TypeMeta, deploymentMetadata *metav1.ObjectMeta) (
	*SidecarInjectionSpec, string, error) {
	spec := &params.pod.Spec
	metadata := &params.pod.ObjectMeta
	meshConfig := params.meshConfig
	// 判断pod中的dns策略是否是 ClusterFirst
	if spec.DNSPolicy != "" && spec.DNSPolicy != corev1.DNSClusterFirst {
		podName := potentialPodName(metadata)
		log.Warnf("%q's DNSPolicy is not %q. The Envoy sidecar may not able to connect to Istio Pilot",
			metadata.Namespace+"/"+podName, corev1.DNSClusterFirst)
	}
	
	// 多集群配置略过
	...

	// 将配置字符串解析到values里面，这里的配置是istio-sidecar-injector cm中 values 的数据内容
	values := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(params.valuesConfig), &values); err != nil {
		...
	}
    
	// 初始化yaml和配置合集的变量
	data := SidecarTemplateData{
		TypeMeta:       typeMetadata,
		DeploymentMeta: deploymentMetadata,
		ObjectMeta:     metadata,
		Spec:           spec,
		ProxyConfig:    meshConfig.GetDefaultConfig(),
		MeshConfig:     meshConfig,
		Values:         values,
	}

	funcMap := template.FuncMap{
		...
	}

	...

	// 使用现有数据填充模板，模板是istio-sidecar-injector cm中 template 的数据内容
	bbuf, err := parseTemplate(params.template, funcMap, data)
	if err != nil {
		return nil, "", err
	}

	// 将buf中的数据解析到 SidecarInjectionSpec 数据结构中
	var sic SidecarInjectionSpec
	if err := yaml.Unmarshal(bbuf.Bytes(), &sic); err != nil {
		...
	}

	// 根据容器 cpu 核数设置并行 https://github.com/istio/istio/issues/11268
	applyConcurrency(sic.Containers)
	// 多集群需要配置
	overwriteClusterInfo(sic.Containers, params)

	// 配置 sidecar 注入状态
	status := &SidecarInjectionStatus{Version: params.version}
	...

	// 配置 sidecar 启动之后在启动业务容器这个特性 https://github.com/istio/istio/pull/24737
	sic.HoldApplicationUntilProxyStarts = meshConfig.DefaultConfig.HoldApplicationUntilProxyStarts.GetValue() ||
		valuesStruct.GetGlobal().GetProxy().GetHoldApplicationUntilProxyStarts().GetValue()

	return &sic, string(statusAnnotationValue), nil
}
```
在上面的方法中，通过用户的配置，将这些配置应用的模板 Yaml 中，从而生成 Sidecar 的注入信息。并且返回注入信息的结构体和注入状态。

`InjectionData`方法调用完成之后，返回到`injectPod`方法，在这个方法中，会调用`createPatch`方法生成要新增的 patch 数据。下面看一下这个方法的代码：
```go
func createPatch(pod *corev1.Pod, prevStatus *SidecarInjectionStatus, revision string, annotations map[string]string,
	sic *SidecarInjectionSpec, workloadName string, mesh *meshconfig.MeshConfig) ([]byte, error) {

	var patch []rfc6902PatchOperation

	rewrite := ShouldRewriteAppHTTPProbers(pod.Annotations, sic)

	sidecar := FindSidecar(sic.Containers)
	// We don't have to escape json encoding here when using golang libraries.
	if rewrite && sidecar != nil {
		if prober := DumpAppProbers(&pod.Spec); prober != "" {
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{Name: status.KubeAppProberEnvName, Value: prober})
		}
	}

	if rewrite {
		patch = append(patch, createProbeRewritePatch(pod.Annotations, &pod.Spec, sic, mesh.GetDefaultConfig().GetStatusPort())...)
	}

	// Remove any containers previously injected by kube-inject using
	// container and volume name as unique key for removal.
	patch = append(patch, removeContainers(pod.Spec.InitContainers, prevStatus.InitContainers, "/spec/initContainers")...)
	patch = append(patch, removeContainers(pod.Spec.Containers, prevStatus.Containers, "/spec/containers")...)
	patch = append(patch, removeVolumes(pod.Spec.Volumes, prevStatus.Volumes, "/spec/volumes")...)
	patch = append(patch, removeImagePullSecrets(pod.Spec.ImagePullSecrets, prevStatus.ImagePullSecrets, "/spec/imagePullSecrets")...)

	if enablePrometheusMerge(mesh, pod.ObjectMeta.Annotations) {
		scrape := status.PrometheusScrapeConfiguration{
			Scrape: pod.ObjectMeta.Annotations["prometheus.io/scrape"],
			Path:   pod.ObjectMeta.Annotations["prometheus.io/path"],
			Port:   pod.ObjectMeta.Annotations["prometheus.io/port"],
		}
		empty := status.PrometheusScrapeConfiguration{}
		if sidecar != nil && scrape != empty {
			by, err := json.Marshal(scrape)
			if err != nil {
				return nil, err
			}
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{Name: status.PrometheusScrapingConfig.Name, Value: string(by)})
		}
		annotations["prometheus.io/port"] = strconv.Itoa(int(mesh.GetDefaultConfig().GetStatusPort()))
		annotations["prometheus.io/path"] = "/stats/prometheus"
		annotations["prometheus.io/scrape"] = "true"
	}

	patch = append(patch, addContainer(sic, pod.Spec.InitContainers, sic.InitContainers, "/spec/initContainers")...)
	patch = append(patch, addContainer(sic, pod.Spec.Containers, sic.Containers, "/spec/containers")...)
	patch = append(patch, addVolume(pod.Spec.Volumes, sic.Volumes, "/spec/volumes")...)
	patch = append(patch, addImagePullSecrets(pod.Spec.ImagePullSecrets, sic.ImagePullSecrets, "/spec/imagePullSecrets")...)

	if sic.DNSConfig != nil {
		patch = append(patch, addPodDNSConfig(sic.DNSConfig, "/spec/dnsConfig")...)
	}

	if pod.Spec.SecurityContext != nil {
		patch = append(patch, addSecurityContext(pod.Spec.SecurityContext, "/spec/securityContext")...)
	}

	patch = append(patch, updateAnnotation(pod.Annotations, annotations)...)

	canonicalSvc, canonicalRev := ExtractCanonicalServiceLabels(pod.Labels, workloadName)
	patchLabels := map[string]string{
		label.TLSMode:                                model.IstioMutualTLSModeLabel,
		model.IstioCanonicalServiceLabelName:         canonicalSvc,
		label.IstioRev:                               revision,
		model.IstioCanonicalServiceRevisionLabelName: canonicalRev,
	}
	if network := topologyValues(sic); network != "" {
		// only added if if not already set
		patchLabels[label.IstioNetwork] = network
	}
	patch = append(patch, addLabels(pod.Labels, patchLabels)...)

	return json.Marshal(patch)
}
```

## 参考
https://www.luozhiyun.com/archives/397

https://www.cnblogs.com/saneri/p/13553979.html

https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/

http://www.ichenfu.com/2018/12/20/k8s-pod-dns-policy/

https://github.com/istio/istio/issues/11268

https://github.com/istio/istio/pull/24737
