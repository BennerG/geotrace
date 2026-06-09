import { useState } from 'react'
import styled from 'styled-components'

interface Props {
  onFetch: (from: string, to: string) => void
  loading: boolean
}

type Preset = '1h' | '24h' | '7d' | '30d' | 'custom'

const PRESETS: { label: string; value: Preset }[] = [
  { label: '1h',  value: '1h' },
  { label: '24h', value: '24h' },
  { label: '7d',  value: '7d' },
  { label: '30d', value: '30d' },
  { label: 'custom', value: 'custom' },
]

function presetWindow(preset: Preset): [string, string] {
  const now = new Date()
  const from = new Date(now)
  switch (preset) {
    case '1h':  from.setHours(now.getHours() - 1); break
    case '24h': from.setDate(now.getDate() - 1); break
    case '7d':  from.setDate(now.getDate() - 7); break
    case '30d': from.setDate(now.getDate() - 30); break
    default:    break
  }
  return [from.toISOString(), now.toISOString()]
}

export function TimeScrubber({ onFetch, loading }: Props) {
  const [active, setActive] = useState<Preset>('1h')
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo]   = useState('')

  const handlePreset = (preset: Preset) => {
    setActive(preset)
    if (preset !== 'custom') {
      const [from, to] = presetWindow(preset)
      onFetch(from, to)
    }
  }

  const handleCustomApply = () => {
    if (!customFrom || !customTo) return
    onFetch(new Date(customFrom).toISOString(), new Date(customTo).toISOString())
  }

  return (
    <Panel>
      <Row>
        <Label>window</Label>
        <Presets>
          {PRESETS.map(p => (
            <PresetBtn
              key={p.value}
              $active={active === p.value}
              onClick={() => handlePreset(p.value)}
            >
              {p.label}
            </PresetBtn>
          ))}
        </Presets>
        {loading && <Spinner />}
      </Row>

      {active === 'custom' && (
        <CustomRow>
          <DateInput
            type="datetime-local"
            value={customFrom}
            onChange={e => setCustomFrom(e.target.value)}
          />
          <Divider>→</Divider>
          <DateInput
            type="datetime-local"
            value={customTo}
            onChange={e => setCustomTo(e.target.value)}
          />
          <ApplyBtn onClick={handleCustomApply}>apply</ApplyBtn>
        </CustomRow>
      )}
    </Panel>
  )
}

const Panel = styled.div`
  position: absolute;
  bottom: 36px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  background: rgba(13, 21, 32, 0.92);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  backdrop-filter: blur(8px);
  min-width: 360px;
`

const Row = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
`

const Label = styled.span`
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  white-space: nowrap;
`

const Presets = styled.div`
  display: flex;
  gap: 4px;
`

const PresetBtn = styled.button<{ $active: boolean }>`
  font-family: var(--font-mono);
  font-size: 11px;
  padding: 3px 9px;
  border-radius: 4px;
  border: 1px solid ${p => p.$active ? 'var(--green)' : 'var(--border)'};
  background: ${p => p.$active ? 'rgba(34,170,114,0.15)' : 'transparent'};
  color: ${p => p.$active ? 'var(--green)' : 'var(--text-dim)'};
  cursor: pointer;
  transition: all 0.15s;

  &:hover {
    border-color: var(--green);
    color: var(--green);
  }
`

const CustomRow = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
`

const DateInput = styled.input`
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  color: var(--text);
  padding: 3px 7px;
  flex: 1;

  &:focus {
    outline: none;
    border-color: var(--green);
  }

  /* remove browser default calendar icon styling */
  color-scheme: dark;
`

const Divider = styled.span`
  color: var(--text-dim);
  font-size: 11px;
`

const ApplyBtn = styled.button`
  font-family: var(--font-mono);
  font-size: 11px;
  padding: 3px 10px;
  border-radius: 4px;
  border: 1px solid var(--green);
  background: rgba(34,170,114,0.15);
  color: var(--green);
  cursor: pointer;
  white-space: nowrap;

  &:hover {
    background: rgba(34,170,114,0.25);
  }
`

const Spinner = styled.div`
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 1.5px solid var(--border);
  border-top-color: var(--green);
  animation: spin 0.7s linear infinite;
  margin-left: auto;

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
`
