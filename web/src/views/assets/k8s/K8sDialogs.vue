<script setup>
defineProps({
  page: {
    type: Object,
    required: true
  }
})
</script>

<template>
  <el-dialog v-model="page.serviceCreateVisible" title="新增服务" width="760px" destroy-on-close><el-form label-position="top" class="service-edit-form"><div class="service-metadata-grid"><el-form-item label="服务名称" required><el-input v-model.trim="page.serviceCreateForm.name" placeholder="例如 orders-api" /></el-form-item><el-form-item label="命名空间" required><el-select v-model="page.serviceCreateForm.namespace"><el-option v-for="item in page.namespaceOptions.filter((option) => option.value !== '__all__')" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item></div><section class="service-edit-section"><div class="service-edit-section-head"><strong>服务类型</strong></div><el-radio-group v-model="page.serviceCreateForm.type" class="service-type-radio-group"><el-radio-button value="ClusterIP">ClusterIP</el-radio-button><el-radio-button value="NodePort">NodePort</el-radio-button><el-radio-button value="LoadBalancer">LoadBalancer</el-radio-button></el-radio-group></section><section class="service-edit-section"><div class="service-edit-section-head"><strong>选择器</strong><span>匹配标签的 Pod 将成为后端端点。</span></div><div class="service-selector-row"><el-input v-model.trim="page.serviceCreateForm.selectorKey" placeholder="标签键" /><el-input v-model.trim="page.serviceCreateForm.selectorValue" placeholder="标签值" /></div></section><section class="service-edit-section"><div class="service-edit-section-head"><strong>端口映射</strong><span>Service 端口暴露访问入口，目标端口指向容器端口。</span></div><div class="service-port-row"><el-input v-model.trim="page.serviceCreateForm.portName" placeholder="端口名称" /><el-input-number v-model="page.serviceCreateForm.port" :min="1" :max="65535" /><el-input v-model.trim="page.serviceCreateForm.targetPort" placeholder="目标端口" /></div></section></el-form><template #footer><el-button @click="page.serviceCreateVisible = false">取消</el-button><el-button type="primary" :loading="page.serviceCreateSaving" @click="page.submitServiceCreate">创建服务</el-button></template></el-dialog>
  <el-dialog v-model="page.ingressCreateVisible" title="新增 Ingress" width="760px" destroy-on-close><el-form label-position="top" class="service-edit-form"><div class="service-metadata-grid"><el-form-item label="Ingress 名称" required><el-input v-model.trim="page.ingressCreateForm.name" placeholder="例如 orders-ingress" /></el-form-item><el-form-item label="命名空间" required><el-select v-model="page.ingressCreateForm.namespace"><el-option v-for="item in page.namespaceOptions.filter((option) => option.value !== '__all__')" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item></div><section class="service-edit-section"><el-form-item label="Ingress Class"><el-input v-model.trim="page.ingressCreateForm.ingressClassName" placeholder="例如 nginx（可选）" /></el-form-item></section><section class="service-edit-section"><div class="service-edit-section-head"><strong>转发规则</strong><span>域名和路径将转发到指定 Service。</span></div><div class="ingress-create-grid"><el-input v-model.trim="page.ingressCreateForm.host" placeholder="域名，例如 api.example.com" /><el-input v-model.trim="page.ingressCreateForm.path" placeholder="路径，例如 /" /><el-select v-model="page.ingressCreateForm.pathType"><el-option label="Prefix" value="Prefix" /><el-option label="Exact" value="Exact" /></el-select><el-input v-model.trim="page.ingressCreateForm.serviceName" placeholder="后端 Service 名称" /><el-input-number v-model="page.ingressCreateForm.servicePort" :min="1" :max="65535" /></div></section></el-form><template #footer><el-button @click="page.ingressCreateVisible = false">取消</el-button><el-button type="primary" :loading="page.ingressCreateSaving" @click="page.submitIngressCreate">创建 Ingress</el-button></template></el-dialog>

  <el-dialog
    v-model="page.serviceEditVisible"
    :title="`编辑服务 · ${page.serviceEditForm.name || '-'}`"
    width="980px"
    class="service-edit-dialog"
    destroy-on-close
  >
    <div v-loading="page.serviceEditLoading" class="service-edit-content">
      <div class="service-edit-summary">
        <div><span>服务名称</span><strong>{{ page.serviceEditForm.name || '-' }}</strong></div>
        <div><span>命名空间</span><strong>{{ page.serviceEditForm.namespace || '-' }}</strong></div>
        <p>以结构化字段维护 Service 定义；保存时仅更新服务路由规则，不会变更工作负载。</p>
      </div>

      <el-form label-position="top" class="service-edit-form">
        <section class="service-edit-section service-metadata-section">
          <div class="service-edit-section-head"><strong>元数据</strong><span>标签用于筛选与关联，注解用于承载控制器或平台扩展配置。</span></div>
          <div class="service-metadata-grid">
            <div class="service-metadata-block">
              <div class="service-metadata-block-head"><strong>标签</strong><el-button link type="primary" @click="page.addServiceMetadataEntry('labels')">+ 添加</el-button></div>
              <div v-if="!page.serviceEditForm.labels.length" class="service-edit-empty">暂无标签</div>
              <div v-else class="service-metadata-list">
                <div v-for="(item, index) in page.serviceEditForm.labels" :key="index" class="service-metadata-row">
                  <el-input v-model.trim="item.key" placeholder="例如 app.kubernetes.io/name" aria-label="标签键" />
                  <el-input v-model.trim="item.value" placeholder="标签值" aria-label="标签值" />
                  <el-button link type="danger" aria-label="删除标签" @click="page.removeServiceMetadataEntry('labels', index)">删除</el-button>
                </div>
              </div>
            </div>
            <div class="service-metadata-block">
              <div class="service-metadata-block-head"><strong>注解</strong><el-button link type="primary" @click="page.addServiceMetadataEntry('annotations')">+ 添加</el-button></div>
              <div v-if="!page.serviceEditForm.annotations.length" class="service-edit-empty">暂无注解</div>
              <div v-else class="service-metadata-list">
                <div v-for="(item, index) in page.serviceEditForm.annotations" :key="index" class="service-metadata-row annotation-metadata-row">
                  <el-input v-model.trim="item.key" placeholder="例如 service.beta.kubernetes.io/..." aria-label="注解键" />
                  <el-input v-model="item.value" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="注解值" aria-label="注解值" />
                  <el-button link type="danger" aria-label="删除注解" @click="page.removeServiceMetadataEntry('annotations', index)">删除</el-button>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section class="service-edit-section">
          <div class="service-edit-section-head"><strong>服务类型</strong><span>决定服务在集群内、集群外或外部 DNS 的访问方式。</span></div>
          <el-radio-group v-model="page.serviceEditForm.type" class="service-type-radio-group">
            <el-radio-button value="ClusterIP">ClusterIP</el-radio-button>
            <el-radio-button value="Headless">Headless</el-radio-button>
            <el-radio-button value="NodePort">NodePort</el-radio-button>
            <el-radio-button value="LoadBalancer">LoadBalancer</el-radio-button>
            <el-radio-button value="ExternalName">ExternalName</el-radio-button>
          </el-radio-group>
          <div v-if="page.serviceEditForm.type === 'Headless'" class="service-headless-option"><div><strong>Headless 服务</strong><span>返回 Pod DNS 记录，不分配 Cluster IP。</span></div></div>
          <el-form-item v-if="page.serviceEditForm.type === 'ExternalName'" label="外部 DNS 名称" required>
            <el-input v-model.trim="page.serviceEditForm.externalName" placeholder="例如 mysql.example.com" />
            <div class="service-edit-hint">ExternalName 不创建代理端点，DNS 将直接别名到此地址。</div>
          </el-form-item>
        </section>

        <section v-if="page.serviceEditForm.type !== 'ExternalName'" class="service-edit-section">
          <div class="service-edit-section-head with-action"><div><strong>选择器</strong><span>匹配标签的 Pod 会成为该服务的后端端点。</span></div><el-button link type="primary" @click="page.addServiceSelector">+ 添加选择器</el-button></div>
          <div v-if="!page.serviceEditForm.selectors.length" class="service-edit-empty">尚未配置选择器；保存后该 Service 不会自动关联 Pod。</div>
          <div v-else class="service-selector-list">
            <div v-for="(item, index) in page.serviceEditForm.selectors" :key="index" class="service-selector-row">
              <el-input v-model.trim="item.key" placeholder="标签键，例如 app" aria-label="选择器标签键" />
              <el-input v-model.trim="item.value" placeholder="标签值，例如 api" aria-label="选择器标签值" />
              <el-button link type="danger" aria-label="删除选择器" @click="page.removeServiceSelector(index)">删除</el-button>
            </div>
          </div>
        </section>

        <section v-if="page.serviceEditForm.type !== 'ExternalName'" class="service-edit-section">
          <div class="service-edit-section-head with-action"><div><strong>端口映射</strong><span>服务端口用于暴露访问入口，目标端口指向容器端口或命名端口。</span></div><el-button link type="primary" @click="page.addServicePort">+ 添加端口</el-button></div>
          <div class="service-port-table-head" :class="{ 'has-node-port': page.serviceEditForm.type === 'NodePort' || page.serviceEditForm.type === 'LoadBalancer' }"><span>名称</span><span>协议</span><span>服务端口</span><span>目标端口</span><span v-if="page.serviceEditForm.type === 'NodePort' || page.serviceEditForm.type === 'LoadBalancer'">NodePort</span><span></span></div>
          <div v-for="(port, index) in page.serviceEditForm.ports" :key="index" class="service-port-row" :class="{ 'has-node-port': page.serviceEditForm.type === 'NodePort' || page.serviceEditForm.type === 'LoadBalancer' }">
            <el-input v-model.trim="port.name" placeholder="例如 http" aria-label="端口名称" />
            <el-select v-model="port.protocol" aria-label="端口协议"><el-option label="TCP" value="TCP" /><el-option label="UDP" value="UDP" /><el-option label="SCTP" value="SCTP" /></el-select>
            <el-input-number v-model="port.port" :min="1" :max="65535" controls-position="right" aria-label="服务端口" />
            <el-input v-model.trim="port.targetPort" placeholder="例如 8080 或 http" aria-label="目标端口" />
            <el-input-number v-if="page.serviceEditForm.type === 'NodePort' || page.serviceEditForm.type === 'LoadBalancer'" v-model="port.nodePort" :min="1" :max="65535" controls-position="right" placeholder="自动分配" aria-label="NodePort" />
            <el-button link type="danger" :disabled="page.serviceEditForm.ports.length === 1" aria-label="删除端口" @click="page.removeServicePort(index)">删除</el-button>
          </div>
        </section>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="page.serviceEditVisible = false">取消</el-button>
      <el-button type="primary" :loading="page.serviceEditSaving" @click="page.submitServiceEdit">保存服务</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.yamlDialogVisible" :title="page.yamlEditor.title" width="1180px" class="yaml-editor-dialog">
    <div class="yaml-workspace">
      <section class="yaml-pane editor">
        <div class="yaml-pane-head">
          <strong>{{ page.t('k8sYamlEditor') }}</strong>
          <span>{{ page.t('k8sYamlTotalLines', { count: page.yamlEditor.yaml.split('\n').length }) }}</span>
        </div>
        <div class="yaml-search-bar">
          <el-input
            v-model="page.yamlSearch.keyword"
            :placeholder="page.t('k8sYamlSearch')"
            clearable
            @input="page.runYAMLSearch(false)"
            @clear="page.runYAMLSearch(false)"
          />
          <span class="yaml-search-summary">
            {{ page.yamlSearch.matches.length ? `${page.yamlSearch.activeIndex + 1}/${page.yamlSearch.matches.length}` : '0/0' }}
          </span>
          <el-button :disabled="!page.yamlSearch.matches.length" @click="page.searchYAMLPrev">{{ page.t('k8sPrevious') }}</el-button>
          <el-button :disabled="!page.yamlSearch.matches.length" @click="page.searchYAMLNext">{{ page.t('k8sNext') }}</el-button>
        </div>
        <div class="yaml-editor-shell">
          <div class="yaml-line-gutter">
            <div class="yaml-line-gutter-inner" :style="{ transform: `translateY(-${page.yamlEditorScrollTop}px)` }">
              <div
                v-for="line in page.yamlLineNumbers"
                :key="line"
                :class="['yaml-line-number', { active: line === page.yamlCurrentLine }]"
              >
                {{ line }}
              </div>
            </div>
          </div>
          <div class="yaml-editor-stage">
            <div class="yaml-current-line" :style="{ top: page.yamlCurrentLineOffset }"></div>
            <textarea
              :ref="(el) => (page.yamlTextareaRef = el)"
              v-model="page.yamlEditor.yaml"
              class="yaml-native-textarea"
              spellcheck="false"
              :placeholder="page.t('k8sEditYamlHere')"
              @input="page.handleYAMLInput"
              @click="page.updateYAMLCurrentLine"
              @keyup="page.updateYAMLCurrentLine"
              @mouseup="page.updateYAMLCurrentLine"
              @scroll="page.handleYAMLScroll"
            ></textarea>
          </div>
        </div>
      </section>
      <section class="yaml-pane preview">
        <div class="yaml-diff-head">
          <strong>{{ page.t('k8sDiffPreview') }}</strong>
          <span>+{{ page.yamlChangeSummary.added }} / -{{ page.yamlChangeSummary.removed }}</span>
        </div>
        <div class="yaml-preview-toolbar">
          <span class="yaml-preview-hint">
            {{ page.yamlChangeSummary.changed ? page.t('k8sChangedLinesHint') : page.t('k8sNoChangesYet') }}
          </span>
        </div>
        <div class="yaml-preview-shell">
          <div class="yaml-line-gutter preview">
            <div class="yaml-line-gutter-inner">
              <div
                v-for="line in page.yamlPreviewLineNumbers"
                :key="`preview-${line}`"
                class="yaml-line-number"
              >
                {{ line }}
              </div>
            </div>
          </div>
          <div class="yaml-diff-panel">
            <div
              v-for="(item, index) in page.yamlDiffLines"
              :key="`${index}-${item.type}`"
              :class="['yaml-diff-line', item.type]"
            >
              <span class="marker">
                {{ item.type === 'added' ? '+' : item.type === 'removed' ? '-' : ' ' }}
              </span>
              <code>{{ item.text || ' ' }}</code>
            </div>
          </div>
        </div>
      </section>
    </div>
    <template #footer>
      <el-button @click="page.yamlDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.yamlSaving" @click="page.submitYAMLUpdate">{{ page.yamlCreating ? '创建' : page.t('save') }}</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.namespaceCreateVisible" :title="page.t('k8sCreateNamespace')" width="480px" destroy-on-close>
    <el-form label-position="top">
      <el-form-item :label="page.t('k8sNamespaceName')" required>
        <el-input v-model="page.namespaceCreateForm.name" maxlength="63" show-word-limit placeholder="例如 game-prod" />
      </el-form-item>
      <div class="dialog-tip">{{ page.t('k8sCreateNamespaceHint') }}</div>
    </el-form>
    <template #footer>
      <el-button @click="page.namespaceCreateVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.namespaceCreateSaving" @click="page.submitNamespaceCreate">{{ page.t('k8sCreate') }}</el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="page.configStorageCreateVisible"
    :title="page.configStorageCreateTitle()"
    width="920px"
    class="config-storage-create-dialog"
    destroy-on-close
  >
    <el-form label-width="96px" class="config-storage-create-form">
      <template v-if="page.configStorageCreateForm.kind === 'pvc'">
        <div class="config-storage-section-head pvc-storage-section-head">
          <strong>存储卷配置</strong>
          <span>先选择存储类，系统会自动限定可用命名空间与读取策略。</span>
        </div>
        <el-form-item label="存储类" required>
          <el-select v-model="page.configStorageCreateForm.storageClass" filterable placeholder="请选择存储类">
            <el-option
              v-for="item in page.pvcStorageClassOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
          <div class="config-storage-field-hint">仅展示平台已创建的存储类；限定命名空间的存储类会自动锁定到对应命名空间。</div>
        </el-form-item>
      </template>

      <el-form-item :label="page.t('k8sNamespace')" required>
        <el-select
          v-model="page.configStorageCreateForm.namespace"
          filterable
          placeholder="请选择命名空间"
          :disabled="page.configStorageEditing || (page.configStorageCreateForm.kind === 'pvc' && page.pvcNamespaceLocked)"
        >
          <el-option
            v-for="item in page.configStorageCreateForm.kind === 'pvc' ? page.pvcNamespaceOptions : page.namespaceOptions.filter((option) => option.value !== '__all__')"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
        <div class="config-storage-field-hint">
          {{ page.configStorageCreateForm.kind === 'pvc' && page.pvcNamespaceLocked
            ? '该存储类已限定命名空间，当前命名空间由存储类自动决定。'
            : 'ConfigMap、Secret 与 PVC 均为命名空间级资源，将只创建到此命名空间。' }}
        </div>
      </el-form-item>

      <el-form-item :label="page.t('k8sName')" required>
        <div class="config-storage-name-wrap">
          <el-input v-model="page.configStorageCreateForm.name" maxlength="63" :disabled="page.configStorageEditing" :placeholder="`请输入 ${page.configStorageCreateForm.kind === 'secret' ? 'Secret' : page.configStorageCreateForm.kind === 'pvc' ? '存储' : 'ConfigMap'} 名称`" />
          <div class="config-storage-field-hint">最长 63 个字符，只能包含小写字母、数字及分隔符（-），且必须以小写字母或数字开头和结尾。</div>
        </div>
      </el-form-item>

      <template v-if="page.configStorageCreateForm.kind === 'pvc'">
        <div class="config-storage-section-head"><strong>申请规格</strong><span>容量由存储卷声明；访问模式严格继承已选存储类，不能手动修改。</span></div>
        <el-row :gutter="16">
          <el-col :span="12"><el-form-item label="存储容量" required><el-input v-model="page.configStorageCreateForm.capacity" placeholder="例如 1Gi" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="访问模式"><el-input :model-value="page.pvcAccessModeLabel" readonly /></el-form-item></el-col>
        </el-row>
      </template>

      <template v-else>
        <el-form-item v-if="page.configStorageCreateForm.kind === 'secret'" label="Secret 类型">
          <el-radio-group v-model="page.configStorageCreateForm.secretType" class="config-storage-radio-group">
            <el-radio-button label="Opaque">Opaque</el-radio-button>
            <el-radio-button label="kubernetes.io/tls">TLS 证书</el-radio-button>
            <el-radio-button label="kubernetes.io/dockerconfigjson">Docker 配置</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <div class="config-storage-section-head">
          <strong>配置项</strong>
          <span>{{ page.configStorageCreateForm.kind === 'secret' ? 'Secret 会以安全的 stringData 方式写入，列表不会展示明文。' : '为 ConfigMap 添加应用配置；支持多行内容。' }}</span>
        </div>
        <el-form-item label="内容" required>
          <div class="config-storage-entry-list">
            <div class="config-storage-entry-table-head"><span>变量名</span><span>变量值</span><span></span></div>
            <div v-for="(entry, index) in page.configStorageCreateForm.entries" :key="index" class="config-storage-entry-row">
              <el-input v-model="entry.key" placeholder="例如 APP_MODE" />
              <el-input v-model="entry.value" type="textarea" :autosize="{ minRows: 6 }" placeholder="请输入变量值，支持多行内容" />
              <el-button link type="danger" :disabled="page.configStorageCreateForm.entries.length === 1" @click="page.removeConfigStorageEntry(index)">删除</el-button>
            </div>
            <el-button link type="primary" @click="page.addConfigStorageEntry">+ 手动添加</el-button>
          </div>
        </el-form-item>
      </template>
    </el-form>
    <template #footer>
      <el-button @click="page.configStorageCreateVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.configStorageCreateSaving" @click="page.submitConfigStorageCreate">
        {{ page.configStorageCreateTitle() }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="page.storageClassCreateVisible"
    title="新增存储类"
    width="840px"
    class="storage-class-create-dialog"
    destroy-on-close
  >
    <div class="storage-class-create-tip">
      创建静态存储资源，创建后可在“存储卷”中填写同名 StorageClass 来声明使用。
    </div>
    <el-form label-position="top" class="storage-class-create-form">
      <section class="storage-class-form-section">
        <div class="storage-class-section-heading">
          <strong>基础配置</strong>
          <span>定义静态卷名称、容量与数据来源。</span>
        </div>
        <el-form-item label="存储类名称" required>
          <el-input v-model.trim="page.storageClassCreateForm.name" maxlength="63" placeholder="例如 game-nfs" />
        </el-form-item>

        <div class="storage-scope-panel">
          <div class="storage-scope-panel-head">
            <el-checkbox v-model="page.storageClassCreateForm.scopeNamespaceEnabled">
              限定命名空间
            </el-checkbox>
            <span>{{ page.storageClassCreateForm.scopeNamespaceEnabled ? '仅指定命名空间可创建存储卷' : '未限定，作用域为集群级' }}</span>
          </div>
          <el-select
            v-if="page.storageClassCreateForm.scopeNamespaceEnabled"
            v-model="page.storageClassCreateForm.scopeNamespace"
            class="storage-scope-namespace-select"
            filterable
            placeholder="请选择命名空间"
          >
            <el-option
              v-for="option in page.namespaceOptions.filter((item) => item.value !== '__all__')"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </div>

        <el-form-item label="存储源类型" required>
          <el-radio-group v-model="page.storageClassCreateForm.sourceType" class="storage-source-type-group">
          <el-radio-button label="hostpath">hostPath · 节点本地路径</el-radio-button>
          <el-radio-button label="nfs">NFS · 远端共享存储</el-radio-button>
        </el-radio-group>
        <div class="config-storage-field-hint">
          {{ page.storageClassCreateForm.sourceType === 'hostpath'
            ? '直接使用节点上的文件系统路径，适用于单节点测试环境；目录会在 Pod 首次实际挂载该存储卷时由节点创建。'
            : '通过 NFS 协议挂载远端存储，支持多节点共享访问。' }}
        </div>
        </el-form-item>

        <el-row :gutter="16" class="storage-class-capacity-row">
        <el-col :span="12">
          <el-form-item label="容量" required>
            <el-input v-model.trim="page.storageClassCreateForm.capacity" placeholder="例如 10Gi" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="回收策略" required>
            <el-radio-group v-model="page.storageClassCreateForm.reclaimPolicy" class="storage-reclaim-policy-group">
              <el-radio-button label="Delete">回收后删除</el-radio-button>
              <el-radio-button label="Retain">回收后保留</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </el-col>
        </el-row>
      </section>

      <section class="storage-class-form-section storage-source-config-section">
        <div class="storage-class-section-heading">
          <strong>挂载配置</strong>
          <span>设置节点或 NFS 的实际存储位置及访问模式。</span>
        </div>
        <div class="storage-source-config-grid">
          <el-form-item v-if="page.storageClassCreateForm.sourceType === 'nfs'" label="NFS 服务地址" required>
            <el-input v-model.trim="page.storageClassCreateForm.nfsServer" placeholder="例如 10.0.0.10" />
          </el-form-item>

          <el-form-item label="路径" required>
            <el-input
              v-model.trim="page.storageClassCreateForm.path"
              :placeholder="page.storageClassCreateForm.sourceType === 'hostpath' ? '例如 /data/k8s' : '例如 /exports/k8s'"
            />
          </el-form-item>

          <el-form-item label="节点访问策略" required>
            <el-select v-model="page.storageClassCreateForm.accessMode">
              <el-option
                v-for="option in page.storageAccessModeOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>
        </div>
      </section>
    </el-form>
    <template #footer>
      <el-button @click="page.storageClassCreateVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.storageClassCreateSaving" @click="page.submitStorageClassCreate">新增存储类</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.scaleDialogVisible" :title="page.t('k8sScaleWorkload')" width="420px">
    <el-form label-width="90px">
      <el-form-item :label="page.t('k8sNamespace')">
        <el-input :model-value="page.scaleForm.namespace" readonly />
      </el-form-item>
      <el-form-item :label="page.t('k8sWorkload')">
        <el-input :model-value="`${page.scaleForm.workloadType} / ${page.scaleForm.workloadName}`" readonly />
      </el-form-item>
      <el-form-item :label="page.t('k8sReplicas')">
        <el-input-number v-model="page.scaleForm.replicas" :min="0" :max="999" controls-position="right" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="page.scaleDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.scaleLoading" @click="page.submitScale">{{ page.t('save') }}</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.batchScaleDialogVisible" title="批量伸缩工作负载" width="460px" destroy-on-close>
    <div class="batch-workload-dialog-tip">将对当前选中的 {{ page.workloadSelectionCount }} 个工作负载统一设置副本数；不支持伸缩的资源会自动跳过。</div>
    <el-form label-position="top">
      <el-form-item label="目标副本数" required>
        <el-input-number v-model="page.batchScaleForm.replicas" :min="0" :max="999" controls-position="right" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="page.batchScaleDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="page.batchScaleSaving" @click="page.submitBatchScale">继续</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.workloadResourceDialogVisible" title="更新 Pod 设置" width="920px" class="workload-resource-dialog-wrap" destroy-on-close>
    <div class="workload-resource-dialog">
      <div class="workload-resource-summary">
        <div><span>命名空间</span><strong>{{ page.workloadResourceForm.namespace }}</strong></div>
        <div><span>工作负载</span><strong>{{ page.workloadResourceForm.workloadName }} · {{ page.workloadResourceForm.workloadType }}</strong></div>
      </div>
      <p class="dialog-tip">可维护每个容器的资源 Request / Limit、镜像拉取策略与环境变量；保存后将更新 Pod 模板并触发工作负载滚动更新。</p>
      <section v-for="container in page.workloadResourceForm.containers" :key="container.name" class="container-resource-card">
        <div class="container-resource-head">
          <strong>{{ container.name }}</strong>
          <span>容器配置</span>
        </div>
        <el-row :gutter="14" class="container-basic-row">
          <el-col :span="15"><el-form-item label="镜像"><el-input :model-value="container.image || '-'" readonly /></el-form-item></el-col>
          <el-col :span="9"><el-form-item label="镜像拉取策略" class="image-pull-policy-field">
            <el-select v-model="container.imagePullPolicy" class="image-pull-policy-select">
              <el-option label="Always（始终拉取）" value="Always" />
              <el-option label="IfNotPresent（本地优先）" value="IfNotPresent" />
              <el-option label="Never（仅本地镜像）" value="Never" />
            </el-select>
          </el-form-item></el-col>
        </el-row>
        <div class="resource-setting-title">CPU / 内存 Request 与 Limit</div>
        <el-row :gutter="14">
          <el-col :span="6"><el-form-item label="CPU Request"><el-input v-model="container.requestCPU" placeholder="100m" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="CPU Limit"><el-input v-model="container.limitCPU" placeholder="1" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="内存 Request"><el-input v-model="container.requestMemory" placeholder="256Mi" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="内存 Limit"><el-input v-model="container.limitMemory" placeholder="1Gi" /></el-form-item></el-col>
        </el-row>
        <div class="resource-setting-title env-setting-title">环境变量</div>
        <div v-if="container.env?.length" class="workload-env-head"><span>变量名</span><span>变量值</span><span>类型</span><span>操作</span></div>
        <div v-if="container.env?.length" class="workload-env-list">
          <div v-for="(env, envIndex) in container.env" :key="`${container.name}-${envIndex}`" class="workload-env-row">
            <el-input v-model="env.name" placeholder="变量名，例如 VECTOR_LOG" />
            <el-input :model-value="env.valueFrom ? (env.source || 'Kubernetes 引用变量') : env.value" :readonly="Boolean(env.valueFrom)" placeholder="变量值" @update:model-value="env.value = $event" />
            <el-tag v-if="env.valueFrom" type="info" effect="plain">引用变量</el-tag>
            <span v-else class="workload-env-type">普通变量</span>
            <el-button link type="danger" @click="page.removeWorkloadEnvironment(container, envIndex)">删除</el-button>
          </div>
        </div>
        <el-button link type="primary" class="add-env-button" @click="page.addWorkloadEnvironment(container)">+ 新增环境变量</el-button>
      </section>
    </div>
    <template #footer>
      <el-button @click="page.workloadResourceDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.workloadResourceSaving" @click="page.submitWorkloadResourceSettings">保存设置</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.imageVersionDialogVisible" :title="page.t('k8sBatchUpdateImageVersion')" width="520px">
    <el-form label-width="110px">
      <el-form-item :label="page.t('k8sSelectedWorkloads')">
        <span>{{ page.workloadSelectionCount }}</span>
      </el-form-item>
      <el-form-item :label="page.t('k8sTargetImageVersion')">
        <el-input v-model="page.imageVersionForm.version" :placeholder="page.t('k8sTargetImageVersionPlaceholder')" />
      </el-form-item>
      <el-form-item>
        <div class="dialog-tip">
          {{ page.t('k8sBatchImageVersionHint') }}
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="page.imageVersionDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.imageVersionSaving" @click="page.submitWorkloadImageVersionUpdate">
        {{ page.t('save') }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="page.istioCreateDialogVisible"
    :title="page.t('k8sCreateIstioResourceTitle', { resource: page.yamlResourceLabel(page.istioCreateForm.resourceType) })"
    width="980px"
  >
    <div class="istio-create-dialog">
      <p class="dialog-tip">{{ page.t('k8sIstioCreateHint') }}</p>
      <el-input
        v-model="page.istioCreateForm.yaml"
        type="textarea"
        :rows="22"
        resize="none"
        class="istio-create-textarea"
      />
    </div>
    <template #footer>
      <el-button @click="page.istioCreateDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.istioCreateSaving" @click="page.submitIstioCreate">
        {{ page.t('save') }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="page.trafficDialogVisible" :title="page.t('k8sAdjustTrafficTitle')" width="640px">
    <div class="traffic-dialog">
      <div class="traffic-dialog-head">
        <div>
          <strong>{{ page.trafficForm.name }}</strong>
          <span>{{ page.trafficForm.namespace }}</span>
        </div>
        <el-tag :type="page.trafficTotalWeight === 100 ? 'success' : 'warning'" effect="light">
          {{ page.t('k8sTrafficTotal', { total: page.trafficTotalWeight }) }}
        </el-tag>
      </div>
      <p class="dialog-tip">{{ page.t('k8sTrafficDialogHint') }}</p>
      <div class="traffic-route-list">
        <div v-for="item in page.trafficForm.routes" :key="item.index" class="traffic-route-item">
          <div class="traffic-route-meta">
            <strong>{{ item.label }}</strong>
            <span>{{ item.host }}</span>
          </div>
          <el-input-number v-model="item.weight" :min="0" :max="100" controls-position="right" />
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="page.trafficDialogVisible = false">{{ page.t('cancel') }}</el-button>
      <el-button type="primary" :loading="page.trafficSaving" @click="page.submitTrafficAdjust">
        {{ page.t('save') }}
      </el-button>
    </template>
  </el-dialog>
</template>
