import React, { useState, useEffect } from 'react'
import { motion } from 'framer-motion'
import { 
  BookOpen, 
  Code, 
  Zap, 
  Users, 
  Settings, 
  Download,
  Github,
  Star,
  Heart,
  GitBranch,
  ChevronRight,
  Upload
} from 'lucide-react'
import Button from '@/components/UI/Button'
import Badge from '@/components/UI/Badge'
import DocumentRenderer from '@/components/Documentation/DocumentRenderer'

interface DocumentationData {
  project: {
    name: string
    version: string
    description: string
    github: string
    license: string
  }
  sections: Array<{
    id: string
    title: string
    icon: string
    description: string
    content: any[]
  }>
}

const Documentation = () => {
  const [activeSection, setActiveSection] = useState('getting-started')
  const [data, setData] = useState<DocumentationData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // 加载文档数据
    import('@/data/documentation.json')
      .then((docData) => {
        setData(docData.default)
        setLoading(false)
      })
      .catch((error) => {
        console.error('Failed to load documentation data:', error)
        setLoading(false)
      })
  }, [])

  const getIcon = (iconName: string) => {
    const icons: { [key: string]: any } = {
      BookOpen,
      Code,
      Zap,
      Users,
      Settings,
      Download,
      Github,
      Star,
      Heart,
      GitBranch,
      Upload
    }
    return icons[iconName] || BookOpen
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950 flex items-center justify-center">
        <div className="text-center">
          <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-4"></div>
          <p className="text-slate-600 dark:text-slate-400">加载中...</p>
        </div>
      </div>
    )
  }

  if (!data) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950 flex items-center justify-center">
        <div className="text-center">
          <BookOpen className="w-12 h-12 text-slate-400 mx-auto mb-4" />
          <h2 className="text-xl font-semibold mb-2 text-slate-900 dark:text-white">加载失败</h2>
          <p className="text-slate-600 dark:text-slate-400">无法加载文档数据，请稍后重试</p>
        </div>
      </div>
    )
  }

  const currentSection = data.sections.find(s => s.id === activeSection)

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      {/* Header */}
      <div className="border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/95 backdrop-blur">
        <div className="max-w-7xl mx-auto px-4 py-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-blue-600 rounded-lg flex items-center justify-center">
                <BookOpen className="w-6 h-6 text-white" />
              </div>
              <div>
                <h1 className="text-2xl font-bold text-slate-900 dark:text-white">{data.project.name} 文档</h1>
                <p className="text-slate-600 dark:text-slate-400">{data.project.description}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-4">
              <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400 flex items-center gap-1">
                <Star className="w-4 h-4" />
                <span>v{data.project.version}</span>
              </Badge>
              <Badge variant="outline" className="flex items-center gap-1">
                <Heart className="w-4 h-4" />
                <span>{data.project.license} 开源</span>
              </Badge>
              <a 
                href={data.project.github} 
                target="_blank" 
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-colors"
              >
                <Github className="w-4 h-4" />
                <span className="text-sm">GitHub</span>
              </a>
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-4 py-8">
        <div className="flex gap-8">
          {/* Sidebar Navigation */}
          <aside className="w-64 flex-shrink-0">
            <nav className="space-y-2 sticky top-8">
              {data.sections.map((section) => {
                const Icon = getIcon(section.icon)
                return (
                  <button
                    key={section.id}
                    onClick={() => setActiveSection(section.id)}
                    className={`w-full flex items-center gap-3 px-4 py-3 rounded-lg text-left transition-colors ${
                      activeSection === section.id
                        ? 'bg-blue-100 dark:bg-blue-900/20 text-blue-900 dark:text-blue-100'
                        : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-slate-800'
                    }`}
                  >
                    <Icon className="w-5 h-5" />
                    <div className="flex-1">
                      <div className="font-medium">{section.title}</div>
                      <div className="text-xs opacity-75">{section.description}</div>
                    </div>
                    {activeSection === section.id && (
                      <ChevronRight className="w-4 h-4" />
                    )}
                  </button>
                )
              })}
            </nav>
          </aside>

          {/* Main Content */}
          <main className="flex-1 min-w-0">
            <motion.div
              key={activeSection}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.3 }}
            >
              <div className="mb-8">
                <div className="flex items-center gap-3 mb-4">
                  <div className="w-10 h-10 bg-blue-100 dark:bg-blue-900/20 rounded-lg flex items-center justify-center">
                    {React.createElement(getIcon(currentSection?.icon || 'BookOpen'), { className: "w-5 h-5 text-blue-600 dark:text-blue-400" })}
                  </div>
                  <div>
                    <h2 className="text-3xl font-bold text-slate-900 dark:text-white">{currentSection?.title}</h2>
                    <p className="text-lg text-slate-600 dark:text-slate-400">{currentSection?.description}</p>
                  </div>
                </div>
                <div className="h-px bg-slate-200 dark:bg-slate-800"></div>
              </div>
              
              <div className="prose prose-slate dark:prose-invert max-w-none">
                {currentSection?.content && (
                  <DocumentRenderer content={currentSection.content} />
                )}
              </div>
            </motion.div>
          </main>
        </div>

        {/* Footer */}
        <div className="mt-16 pt-8 border-t border-slate-200 dark:border-slate-800">
          <div className="flex items-center justify-between">
            <div className="text-sm text-slate-600 dark:text-slate-400">
              <p>© {new Date().getFullYear()} {data.project.name}. {data.project.license} License.</p>
            </div>
            <div className="flex items-center gap-4">
              <Button variant="outline" size="sm" className="flex items-center gap-1">
                <Download className="w-4 h-4" />
                <span>下载源码</span>
              </Button>
              <Button size="sm" className="flex items-center gap-1">
                <Github className="w-4 h-4" />
                <span>参与贡献</span>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export default Documentation