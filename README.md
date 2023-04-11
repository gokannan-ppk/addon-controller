# addon-controller

addon-controller是一个面向k8s集群插件（addon）的自动化交付工具。

## 目标
addon-controller的目标是从统一制品仓获取对应的Addon插件包，根据其中的meta.json元文件向CCE服务注册插件版本信息，并通过deployment启动一个自研helm客户端，该客户端封装了helm常用命令，可以用于上传helm chart到对应的helm仓库。

## 注意点
1）注意级联资源的维护，通过对finalizer判断中相关字段的判断实现，因此要维护好finalizer字段。
2）级联资源的删除和创建都是异步的，需要设置检测超时的逻辑，如果检测超时，则需要直接return，并重新入队等待下一次处理。
3）pre-delete hook需要保证幂等。

## 思考
基于以上addon-controller的实现逻辑，可以进一步实现一个更通用的helm chart交付工具，可以做出以下定义：
1）Helm chart资源：维护chart的状态实现自动化操作。
2）Helm repo资源：维护repo资源用于部署构建helm仓库。
3）Helm client资源：维护client用于封装helm命令行并提供接口给controller使用。
4）Cluster资源：维护一个目标k8s集群，通过封装的helm客户端向目标集群部署应用。
