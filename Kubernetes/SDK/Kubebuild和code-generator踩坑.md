# Kubebuild和code-generator踩坑

## 概览


kubebuilder和[k8s.io/code-generator](https://link.segmentfault.com/?enc=ozdTFTWF2ddTB5b67xNxFA==.xoriI3ITXoODTVJ7h9EGDSEA/t2bqPDo7Ey4fBLgfIPKOpN5WjdmowXCgGK+mpyC)类似，是一个码生成工具，用于为你的CRD生成[kubernetes-style API](https://link.segmentfault.com/?enc=T5TDqbgoEUhhRWOXGH4OlA==.TzktAA3pGVcPln7R+b+xE3UAKrxTEOp4kR6LkyJx0JQJXIqmmN1nrnseE/j6ySivDx0SrxZMt9hlhc8nv0irwaR4Bz/g1oJMudyHtwZHrSLuqkfm219zLHk/HSnFRk54lti2x+a87ZsgaXddEAalhA==)实现。目前个人使用的方式时Kubebuilder生成CRD和manifests yaml，再使用code-generator生成informers、listers、clientsets。注意，本文中所讲的方法不能再Windows环境下使用。

本文中使用的Kubebuilder版本为3.2.0，code-generator版本为v0.22.1

## 一、使用kubebuilder创建项目

**注意：** Kubebuilder创建项目的时候，对项目中的文件内容有着严格的要求，所以最好是从一个新的项目开始创建。如果是从一个老的项目开始创建，那么需要花很大功夫。

首先下载Kubebuilder：

```Bash
# download kubebuilder and install locally.
curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder && mv kubebuilder /usr/local/bin/

```


创建一个go mod项目，进入到项目中执行：

```Bash
kubebuilder init --domain zsy.com　
kubebuilder edit --multigroup=true
```


## 二、生成Resource和manifests

```Bash
kubebuilder create api --group webapp --version v1 --kind Guestbook
Create Resource [y/n]
y
Create Controller [y/n]
n
```


如果在这一步遇到了错误，有可能是gcc的问题，需要安装或者升级一下gcc。

执行完这一步之后，就可以修改`api/v1/groupversion_info.go` 文件，将这个文件中的CRD配置修改为需要的配置字段，修改完成之后，执行make命令来更新代码。

添加文件`api/v1/rbac.go`，这个文件用生成RBAC manifests：

```Go
// +kubebuilder:rbac:groups=webapp.example.com,resources=guestbooks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webapp.example.com,resources=guestbooks/status,verbs=get;update;patch
 
package v1
```


然后生成CRD manifests：

```Bash
make manifests
```


## 三、使用code-generator

### 1）准备脚本

在hack目录下添加以下文件：

```纯文本
└── hack
    ├── tools.go
    ├── update-codegen.sh
    └── verify-codegen.sh
```


新建`hack/tools.go`文件：

```Go
// +build tools
package tools
```


新建`hack/update-codegen.sh`，注意修改几个变量：


- `MODULE`和`go.mod`保持一致

- `API_PKG=api`，和`api`目录保持一致

- `OUTPUT_PKG=generated/webapp`，生成Resource时指定的group一样

- `GROUP_VERSION=webapp:v1`和生成Resource时指定的group version对应

```Bash
#!/usr/bin/env bash
 
set -o errexit
set -o nounset
set -o pipefail
 
# corresponding to go mod init <module>
MODULE=foo-controller
# api package
APIS_PKG=api
# generated output package
OUTPUT_PKG=generated/webapp
# group-version such as foo:v1alpha1
GROUP_VERSION=webapp:v1
 
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}
 
# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash "${CODEGEN_PKG}"/generate-groups.sh "client,lister,informer" \
  ${MODULE}/${OUTPUT_PKG} ${MODULE}/${APIS_PKG} \
  ${GROUP_VERSION} \
  --go-header-file "${SCRIPT_ROOT}"/hack/boilerplate.go.txt \
  --output-base "${SCRIPT_ROOT}

```


新建`hack/verify-codegen.sh`

`OUTPUT_PKG=generated/webapp`，生成Resource时指定的group一样

`MODULE`是`domain`和`go.mod`的结合

```Bash
#!/usr/bin/env bash
 
set -o errexit
set -o nounset
set -o pipefail
 
OUTPUT_PKG=generated/webapp
MODULE=zsy.com/foo-controller
 
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
 
DIFFROOT="${SCRIPT_ROOT}/${OUTPUT_PKG}"
TMP_DIFFROOT="${SCRIPT_ROOT}/_tmp/${OUTPUT_PKG}"
_tmp="${SCRIPT_ROOT}/_tmp"
 
cleanup() {
  rm -rf "${_tmp}"
}
trap "cleanup" EXIT SIGINT
 
cleanup
 
mkdir -p "${TMP_DIFFROOT}"
cp -a "${DIFFROOT}"/* "${TMP_DIFFROOT}"
 
"${SCRIPT_ROOT}/hack/update-codegen.sh"
echo "copying generated ${SCRIPT_ROOT}/${MODULE}/${OUTPUT_PKG} to ${DIFFROOT}"
cp -r "${SCRIPT_ROOT}/${MODULE}/${OUTPUT_PKG}"/* "${DIFFROOT}"
 
echo "diffing ${DIFFROOT} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT}" "${TMP_DIFFROOT}" || ret=$?
cp -a "${TMP_DIFFROOT}"/* "${DIFFROOT}"
if [[ $ret -eq 0 ]]
then
  echo "${DIFFROOT} up to date."
else
  echo "${DIFFROOT} is out of date. Please run hack/update-codegen.sh"
  exit 1
fi
```


### 2）更新依赖

编辑项目的`go.mod`文件

```Go
module foo-controller
 
go 1.16
 
require (
    github.com/go-logr/logr v0.1.0
    github.com/onsi/ginkgo v1.11.0
    github.com/onsi/gomega v1.8.1
    k8s.io/apimachinery v0.22.1
    k8s.io/client-go v0.22.1
    k8s.io/code-generator v0.22.1
    sigs.k8s.io/controller-runtime v0.6.0
)
```


然后使用`vend`工具更新vendor，注意这里不要用`go mod vendor`命令来更新，因为k8s.io/code-generator这个依赖在项目中并没有真正被引用过，所以使用`go mod vendor`是无法将这个依赖更新到vendor中，要借助第三方工具vend来实现。[https://github.com/nomad-software/vend](https://github.com/nomad-software/vend)

使用命令`go get `[github.com/nomad-software/vend](http://github.com/nomad-software/vend)来安装，然后再项目根目录下执行`vend`命令。

然后给`generate-groups.sh`添加可执行权限：

```Bash
  chmod +x vendor/k8s.io/code-generator/generate-groups.sh
```


### 3）修改代码结构

code-generator对代码结构有着一定的要求，但是kubebuild工具生成的结构并不符合这个要求，所以我们要对项目的代码结构进行修改。

原结构：

```Bash
├── api
│   └── v1
│       ├── groupversion_info.go
│       ├── guestbook_types.go
│       └── zz_generated.deepcopy.go
```


修改后结构：

```纯文本
├── api
│   └── webapp
│       └── v1
│           ├── groupversion_info.go
│           ├── guestbook_types.go
│           └── zz_generated.deepcopy.go
```


也就是中间添加一层路径，名称为组名。

### 4）添加注释

修改`guestbook_types.go`文件，添加上tag `// +genclient`，这个注释内容要加在k8s对象结构体上面，要注意检查添加的位置是否正确。

```Go
// +genclient
// +kubebuilder:object:root=true
 
// Guestbook is the Schema for the guestbooks API
type Guestbook struct {
```


新建`api/webapp/v1/doc.go`，注意`// +groupName=webapp.zsy.com`：

```Go
// +groupName=webapp.zsy.com
 
package v1
```


新建`api/webapp/v1/register.go`，code generator生成的代码需要用到它：

```Go
package v1
 
import (
    "k8s.io/apimachinery/pkg/runtime/schema"
)
 
// SchemeGroupVersion is group version used to register these objects.
var SchemeGroupVersion = GroupVersion
 
func Resource(resource string) schema.GroupResource {
    return SchemeGroupVersion.WithResource(resource).GroupResource()
}
```


执行hack/update-codegen.sh：

```Bash
./hack/update-codegen.sh
```


会得到`zsy.com/foo-controller`目录：

```纯文本
foo-controller
└── generated
    └── webapp
        ├── clientset
        ├── informers
        └── listers
```


到此为止，代码生成的步骤就执行完成了。

## 参考文档

[https://www.jianshu.com/p/82df84131f78](https://www.jianshu.com/p/82df84131f78)

[https://www.cnblogs.com/zhangmingcheng/p/15405022.html](https://www.cnblogs.com/zhangmingcheng/p/15405022.html)

