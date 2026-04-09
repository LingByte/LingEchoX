import { useState } from 'react'
import { motion } from 'framer-motion'
import { Copy, Check, Code2, FileJson, Package } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import { cn } from '@/utils/cn'

const SDKDocumentation = () => {
  const [copiedCode, setCopiedCode] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'js' | 'python' | 'go' | 'curl'>('js')

  const copyToClipboard = (code: string, id: string) => {
    navigator.clipboard.writeText(code)
    setCopiedCode(id)
    setTimeout(() => setCopiedCode(null), 2000)
  }

  const CodeBlock = ({ code, language = 'bash', id }: { code: string; language?: string; id: string }) => (
    <div className="relative bg-slate-900 dark:bg-slate-950 rounded-lg overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 bg-slate-800 dark:bg-slate-900 border-b border-slate-700">
        <span className="text-xs font-medium text-slate-400">{language}</span>
        <button
          onClick={() => copyToClipboard(code, id)}
          className="flex items-center gap-2 px-2 py-1 rounded text-xs text-slate-400 hover:text-slate-200 hover:bg-slate-700 transition-colors"
        >
          {copiedCode === id ? (
            <>
              <Check className="w-4 h-4" />
              已复制
            </>
          ) : (
            <>
              <Copy className="w-4 h-4" />
              复制
            </>
          )}
        </button>
      </div>
      <pre className="p-4 overflow-x-auto text-sm text-slate-100 font-mono">
        <code>{code}</code>
      </pre>
    </div>
  )

  const TabButtons = ({ tabs }: { tabs: Array<{ id: string; label: string }> }) => (
    <div className="flex gap-2 mb-4 border-b border-slate-200 dark:border-slate-700">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => setActiveTab(tab.id as any)}
          className={cn(
            'px-4 py-2 text-sm font-medium border-b-2 transition-colors',
            activeTab === tab.id
              ? 'border-blue-600 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-300'
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )

  return (
    <AdminLayout title="SDK 文档" description="七牛云联络中心 API 文档和集成指南">
      <div className="space-y-6 animate-in fade-in-0 slide-in-from-bottom-4 duration-500">
        {/* 概述 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">概述</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">
              七牛云联络中心 SDK 提供了简单易用的 API 接口，让您可以轻松集成文件存储、上传、下载等功能到您的应用中。
            </p>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Card className="p-4">
                <div className="flex items-center gap-3 mb-2">
                  <Package className="w-5 h-5 text-blue-600 dark:text-blue-400" />
                  <h4 className="font-semibold">简单易用</h4>
                </div>
                <p className="text-sm text-slate-600 dark:text-slate-400">提供直观的 API 设计，快速上手</p>
              </Card>
              <Card className="p-4">
                <div className="flex items-center gap-3 mb-2">
                  <Code2 className="w-5 h-5 text-green-600 dark:text-green-400" />
                  <h4 className="font-semibold">多语言支持</h4>
                </div>
                <p className="text-sm text-slate-600 dark:text-slate-400">支持 JavaScript、Python、Go 等多种语言</p>
              </Card>
              <Card className="p-4">
                <div className="flex items-center gap-3 mb-2">
                  <FileJson className="w-5 h-5 text-purple-600 dark:text-purple-400" />
                  <h4 className="font-semibold">RESTful API</h4>
                </div>
                <p className="text-sm text-slate-600 dark:text-slate-400">基于标准 HTTP 协议，易于集成</p>
              </Card>
            </div>
          </Card>
        </motion.div>

        {/* 安装 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">安装</h2>
            <div className="space-y-4">
              <div>
                <h4 className="font-semibold mb-2">JavaScript / Node.js</h4>
                <CodeBlock code={`npm install lingstorage-sdk\nyarn add lingstorage-sdk`} language="bash" id="install-js" />
              </div>
              <div>
                <h4 className="font-semibold mb-2">Python</h4>
                <CodeBlock code={`pip install lingstorage-sdk\npoetry add lingstorage-sdk`} language="bash" id="install-python" />
              </div>
              <div>
                <h4 className="font-semibold mb-2">Go</h4>
                <CodeBlock code={`go get github.com/lingstorage/sdk-go`} language="bash" id="install-go" />
              </div>
            </div>
          </Card>
        </motion.div>

        {/* 认证 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">认证</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">所有 API 请求都需要进行身份验证。</p>
            <TabButtons tabs={[{ id: 'js', label: 'JavaScript' }, { id: 'python', label: 'Python' }, { id: 'go', label: 'Go' }, { id: 'curl', label: 'cURL' }]} />
            {activeTab === 'js' && <CodeBlock code={`import { LingStorage } from 'lingstorage-sdk'\nconst client = new LingStorage({\n  apiKey: 'your_api_key_here',\n  baseURL: 'https://api.example.com'\n})`} language="javascript" id="auth-js" />}
            {activeTab === 'python' && <CodeBlock code={`from lingstorage import LingStorage\nclient = LingStorage(\n    api_key='your_api_key_here',\n    base_url='https://api.example.com'\n)`} language="python" id="auth-python" />}
            {activeTab === 'go' && <CodeBlock code={`package main\nimport "github.com/lingstorage/sdk-go"\nclient := lingstorage.NewClient(\n    "your_api_key_here",\n    "https://api.example.com",\n)`} language="go" id="auth-go" />}
            {activeTab === 'curl' && <CodeBlock code={`curl -H "Authorization: Bearer YOUR_API_KEY" \\\n  https://api.example.com/api/files`} language="bash" id="auth-curl" />}
          </Card>
        </motion.div>

        {/* 文件上传 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">文件上传</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">上传文件到存储服务。支持单文件上传和批量上传。</p>
            <TabButtons tabs={[{ id: 'js', label: 'JavaScript' }, { id: 'python', label: 'Python' }, { id: 'go', label: 'Go' }, { id: 'curl', label: 'cURL' }]} />
            {activeTab === 'js' && (
              <div className="space-y-4">
                <div>
                  <h4 className="font-semibold mb-2">基础上传</h4>
                  <CodeBlock code={`const file = document.getElementById('fileInput').files[0]\nconst response = await client.upload({\n  file: file,\n  bucket: 'default',\n  key: 'uploads/my-file.jpg'\n})\nconsole.log('上传成功:', response.url)`} language="javascript" id="upload-js-basic" />
                </div>
                <div>
                  <h4 className="font-semibold mb-2">带进度条的上传</h4>
                  <CodeBlock code={`const response = await client.upload({\n  file: file,\n  bucket: 'default',\n  onProgress: (progress) => {\n    console.log(\`上传进度: \${progress.percent}%\`)\n  }\n})`} language="javascript" id="upload-js-progress" />
                </div>
              </div>
            )}
            {activeTab === 'python' && (
              <div className="space-y-4">
                <div>
                  <h4 className="font-semibold mb-2">基础上传</h4>
                  <CodeBlock code={`with open('image.jpg', 'rb') as f:\n    response = client.upload(\n        file=f,\n        bucket='default',\n        key='uploads/my-file.jpg'\n    )\nprint('上传成功:', response['url'])`} language="python" id="upload-python-basic" />
                </div>
                <div>
                  <h4 className="font-semibold mb-2">带进度条的上传</h4>
                  <CodeBlock code={`def progress_callback(current, total):\n    percent = (current / total) * 100\n    print(f'上传进度: {percent:.1f}%')\n\nwith open('image.jpg', 'rb') as f:\n    response = client.upload(\n        file=f,\n        bucket='default',\n        progress_callback=progress_callback\n    )`} language="python" id="upload-python-progress" />
                </div>
              </div>
            )}
            {activeTab === 'go' && <CodeBlock code={`file, err := os.Open("image.jpg")\ndefer file.Close()\nresponse, err := client.Upload(context.Background(), &lingstorage.UploadRequest{\n    File:   file,\n    Bucket: "default",\n    Key:    "uploads/my-file.jpg",\n})\nfmt.Println("上传成功:", response.URL)`} language="go" id="upload-go-basic" />}
            {activeTab === 'curl' && <CodeBlock code={`curl -X POST https://api.example.com/api/upload \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -F "file=@image.jpg" \\\n  -F "bucket=default"`} language="bash" id="upload-curl" />}
          </Card>
        </motion.div>

        {/* 文件下载 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">文件下载</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">下载已上传的文件。</p>
            <TabButtons tabs={[{ id: 'js', label: 'JavaScript' }, { id: 'python', label: 'Python' }, { id: 'go', label: 'Go' }, { id: 'curl', label: 'cURL' }]} />
            {activeTab === 'js' && (
              <div className="space-y-4">
                <div>
                  <h4 className="font-semibold mb-2">获取文件 URL</h4>
                  <CodeBlock code={`const response = await client.getFileUrl({\n  key: 'uploads/my-file.jpg',\n  expiresIn: 3600\n})\nconsole.log('文件 URL:', response.url)`} language="javascript" id="download-js-url" />
                </div>
                <div>
                  <h4 className="font-semibold mb-2">直接下载文件</h4>
                  <CodeBlock code={`const response = await client.download({\n  key: 'uploads/my-file.jpg'\n})\nconst blob = await response.blob()\nconst url = window.URL.createObjectURL(blob)\nconst a = document.createElement('a')\na.href = url\na.download = 'my-file.jpg'\na.click()`} language="javascript" id="download-js-direct" />
                </div>
              </div>
            )}
            {activeTab === 'python' && (
              <div className="space-y-4">
                <div>
                  <h4 className="font-semibold mb-2">获取文件 URL</h4>
                  <CodeBlock code={`response = client.get_file_url(\n    key='uploads/my-file.jpg',\n    expires_in=3600\n)\nprint('文件 URL:', response['url'])`} language="python" id="download-python-url" />
                </div>
                <div>
                  <h4 className="font-semibold mb-2">直接下载文件</h4>
                  <CodeBlock code={`response = client.download(key='uploads/my-file.jpg')\nwith open('downloaded-file.jpg', 'wb') as f:\n    f.write(response.content)\nprint('文件已下载')`} language="python" id="download-python-direct" />
                </div>
              </div>
            )}
            {activeTab === 'go' && <CodeBlock code={`response, err := client.GetFileURL(context.Background(), &lingstorage.GetFileURLRequest{\n    Key:       "uploads/my-file.jpg",\n    ExpiresIn: 3600,\n})\nfmt.Println("文件 URL:", response.URL)`} language="go" id="download-go-url" />}
            {activeTab === 'curl' && <CodeBlock code={`curl -X GET "https://api.example.com/api/file/url?key=uploads/my-file.jpg" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`} language="bash" id="download-curl-url" />}
          </Card>
        </motion.div>

        {/* 文件删除 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">文件删除</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">删除已上传的文件。</p>
            <TabButtons tabs={[{ id: 'js', label: 'JavaScript' }, { id: 'python', label: 'Python' }, { id: 'go', label: 'Go' }, { id: 'curl', label: 'cURL' }]} />
            {activeTab === 'js' && <CodeBlock code={`const response = await client.delete({\n  key: 'uploads/my-file.jpg'\n})\nconsole.log('删除成功:', response.message)`} language="javascript" id="delete-js" />}
            {activeTab === 'python' && <CodeBlock code={`response = client.delete(key='uploads/my-file.jpg')\nprint('删除成功:', response['message'])`} language="python" id="delete-python" />}
            {activeTab === 'go' && <CodeBlock code={`response, err := client.Delete(context.Background(), &lingstorage.DeleteRequest{\n    Key: "uploads/my-file.jpg",\n})\nfmt.Println("删除成功:", response.Message)`} language="go" id="delete-go" />}
            {activeTab === 'curl' && <CodeBlock code={`curl -X DELETE https://api.example.com/api/file \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"key": "uploads/my-file.jpg"}'`} language="bash" id="delete-curl" />}
          </Card>
        </motion.div>

        {/* 错误处理 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">错误处理</h2>
            <p className="text-slate-600 dark:text-slate-400 mb-4">API 返回标准的 HTTP 状态码和错误信息。</p>
            <TabButtons tabs={[{ id: 'js', label: 'JavaScript' }, { id: 'python', label: 'Python' }, { id: 'go', label: 'Go' }]} />
            {activeTab === 'js' && <CodeBlock code={`try {\n  const response = await client.upload({ file, bucket: 'default' })\n  console.log('上传成功:', response.url)\n} catch (error) {\n  if (error.code === 400) {\n    console.error('请求参数错误:', error.message)\n  } else if (error.code === 401) {\n    console.error('认证失败:', error.message)\n  } else if (error.code === 413) {\n    console.error('文件过大:', error.message)\n  }\n}`} language="javascript" id="error-js" />}
            {activeTab === 'python' && <CodeBlock code={`from lingstorage.exceptions import LingStorageError\ntry:\n    response = client.upload(file=f, bucket='default')\nexcept LingStorageError as e:\n    if e.code == 400:\n        print('请求参数错误:', e.message)\n    elif e.code == 401:\n        print('认证失败:', e.message)\n    elif e.code == 413:\n        print('文件过大:', e.message)`} language="python" id="error-python" />}
            {activeTab === 'go' && <CodeBlock code={`response, err := client.Upload(context.Background(), &lingstorage.UploadRequest{\n    File:   file,\n    Bucket: "default",\n})\nif err != nil {\n    if apiErr, ok := err.(*lingstorage.APIError); ok {\n        switch apiErr.Code {\n        case 400:\n            log.Println("请求参数错误:", apiErr.Message)\n        case 401:\n            log.Println("认证失败:", apiErr.Message)\n        }\n    }\n}`} language="go" id="error-go" />}
            <div className="mt-4 space-y-2">
              <h4 className="font-semibold">常见错误码</h4>
              <div className="space-y-2">
                <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                  <p className="font-semibold text-red-900 dark:text-red-200">400 Bad Request</p>
                  <p className="text-sm text-red-800 dark:text-red-300">请求参数错误或文件大小超限</p>
                </div>
                <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                  <p className="font-semibold text-red-900 dark:text-red-200">401 Unauthorized</p>
                  <p className="text-sm text-red-800 dark:text-red-300">API Key 无效或过期</p>
                </div>
                <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                  <p className="font-semibold text-red-900 dark:text-red-200">404 Not Found</p>
                  <p className="text-sm text-red-800 dark:text-red-300">文件或资源不存在</p>
                </div>
                <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                  <p className="font-semibold text-red-900 dark:text-red-200">429 Too Many Requests</p>
                  <p className="text-sm text-red-800 dark:text-red-300">请求过于频繁，请稍后重试</p>
                </div>
              </div>
            </div>
          </Card>
        </motion.div>

        {/* 最佳实践 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-4">最佳实践</h2>
            <div className="space-y-3">
              <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">✓ 安全存储 API Key</h4>
                <p className="text-sm text-blue-800 dark:text-blue-300">不要在客户端代码中硬编码 API Key，使用环境变量或配置文件存储。</p>
              </div>
              <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">✓ 实现重试机制</h4>
                <p className="text-sm text-blue-800 dark:text-blue-300">对于网络请求，实现指数退避重试策略以处理临时故障。</p>
              </div>
              <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">✓ 验证文件类型</h4>
                <p className="text-sm text-blue-800 dark:text-blue-300">在上传前验证文件类型和大小，提供更好的用户体验。</p>
              </div>
              <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">✓ 使用 HTTPS</h4>
                <p className="text-sm text-blue-800 dark:text-blue-300">始终使用 HTTPS 协议进行 API 调用，确保数据传输安全。</p>
              </div>
            </div>
          </Card>
        </motion.div>

        {/* 支持信息 */}
        <motion.div initial={{ opacity: 0, y: 20 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}>
          <Card className="p-6 bg-gradient-to-r from-blue-50 to-indigo-50 dark:from-blue-900/20 dark:to-indigo-900/20 border-blue-200 dark:border-blue-800">
            <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-2">需要帮助？</h3>
            <p className="text-slate-600 dark:text-slate-400 mb-4">如果您在集成过程中遇到问题，请查看我们的常见问题解答或联系技术支持。</p>
            <div className="flex gap-3">
              <Button variant="primary" size="sm">查看常见问题</Button>
              <Button variant="ghost" size="sm">联系支持</Button>
            </div>
          </Card>
        </motion.div>
      </div>
    </AdminLayout>
  )
}

export default SDKDocumentation
