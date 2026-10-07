import { useState } from 'react'
import { Copy, Check } from 'lucide-react'

export default function CopyableId({ id }: { id: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    navigator.clipboard.writeText(id)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <span className="inline-flex items-center gap-1.5 group cursor-pointer" onClick={handleCopy}>
      <span className="font-mono text-sm text-gray-900">{id.slice(0, 12)}...</span>
      <span className="opacity-0 group-hover:opacity-100 transition-opacity">
        {copied
          ? <Check size={14} className="text-green-500" />
          : <Copy size={14} className="text-gray-400 hover:text-gray-600" />}
      </span>
    </span>
  )
}
