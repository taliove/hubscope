// Protocol tag type mapping (GH #34, spec 0014; ui-guidelines §5 protocol tag
// entry). Protocol tag semantics is "contract family distinction", NOT
// health — the tag never uses failing/danger, and success/warning here read
// as the two chat contract families' distinction colors in the protocol-word
// context, not as endpoint status colors (§3 vocabulary stays status-only).
import type { TagProps } from 'element-plus'
import type { Protocol } from '@/api/types'

// el-tag `type` accepts 'primary' | 'success' | 'warning' | 'danger' | 'info'.
// anthropic/openai keep their existing colors (issue AC); the image
// protocols are neutral info so they neither claim the red/yellow status
// semantics nor get confused with the chat families.
const PROTOCOL_TAG_TYPES: Record<Protocol, TagProps['type']> = {
  anthropic: 'success',
  openai: 'warning',
  images_generation: 'info',
  images_edit: 'info',
}

// protocolTagType returns the el-tag `type` for a protocol. Unknown/null
// protocols fall back to 'info' (neutral) — defensive, and automatically
// compatible with future protocols (e.g. images_edit landed before its
// backend endpoints existed).
export function protocolTagType(protocol: string | null | undefined): TagProps['type'] {
  if (protocol && protocol in PROTOCOL_TAG_TYPES) {
    return PROTOCOL_TAG_TYPES[protocol as Protocol]
  }
  return 'info'
}
