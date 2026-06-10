import styled, { keyframes } from 'styled-components'
import { StatsResponse } from '../types'

interface Props {
  stats: StatsResponse | null
  error: boolean
}

export function StatsBar({ stats, error }: Props) {
  const topCountries = stats
    ? Object.entries(stats.countries)
        .filter(([code]) => code !== 'XX')
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5)
    : []

  const unknownCount = stats?.countries['XX'] ?? 0

  return (
    <Panel>
      <SectionLabel>traffic</SectionLabel>

      <StatRow>
        <StatValue>{error ? '—' : (stats?.total_in_window ?? '…')}</StatValue>
        <StatLabel>total requests</StatLabel>
      </StatRow>

      <StatRow>
        <StatValue>{error ? '—' : (stats?.req_per_min ?? '…')}</StatValue>
        <StatLabel>req / min</StatLabel>
      </StatRow>

      <Divider />

      <SectionLabel>top countries</SectionLabel>

      {topCountries.length === 0 && !error && (
        <EmptyLabel>no geo data yet</EmptyLabel>
      )}

      {topCountries.map(([code, count]) => (
        <CountryRow key={code}>
          <Flag>{countryFlag(code)}</Flag>
          <CountryCode>{code}</CountryCode>
          <Bar>
            <BarFill
              $pct={topCountries[0][1] > 0 ? (count / topCountries[0][1]) * 100 : 0}
            />
          </Bar>
          <Count>{count}</Count>
        </CountryRow>
      ))}

      {unknownCount > 0 && (
        <CountryRow>
          <Flag>🌐</Flag>
          <CountryCode>XX</CountryCode>
          <Bar>
            <BarFill
              $pct={topCountries[0]?.[1] > 0 ? (unknownCount / topCountries[0][1]) * 100 : 100}
            />
          </Bar>
          <Count>{unknownCount}</Count>
        </CountryRow>
      )}

      <Divider />
      <PollLabel>
        <PulseDot />
        updating every 15s
      </PollLabel>
    </Panel>
  )
}

const Panel = styled.div`
  position: absolute;
  top: 50px;
  right: 12px;
  z-index: 10;
  background: rgba(13, 21, 32, 0.92);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  backdrop-filter: blur(8px);
  min-width: 180px;
  max-width: 200px;
`

const SectionLabel = styled.div`
  font-family: var(--font-mono);
  font-size: 9px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.1em;
  margin-bottom: 2px;
`

const StatRow = styled.div`
  display: flex;
  align-items: baseline;
  gap: 6px;
`

const StatValue = styled.span`
  font-family: var(--font-mono);
  font-size: 20px;
  font-weight: 600;
  color: var(--green);
  line-height: 1;
`

const StatLabel = styled.span`
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-dim);
`

const Divider = styled.div`
  height: 1px;
  background: var(--border);
  margin: 2px 0;
`

const CountryRow = styled.div`
  display: flex;
  align-items: center;
  gap: 6px;
`

const Flag = styled.span`
  font-size: 13px;
  line-height: 1;
  width: 16px;
`

const CountryCode = styled.span`
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-dim);
  width: 20px;
`

const Bar = styled.div`
  flex: 1;
  height: 3px;
  background: var(--border);
  border-radius: 2px;
  overflow: hidden;
`

const BarFill = styled.div<{ $pct: number }>`
  height: 100%;
  width: ${p => p.$pct}%;
  background: var(--green);
  border-radius: 2px;
  transition: width 0.4s ease;
`

const Count = styled.span`
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text);
  width: 24px;
  text-align: right;
`

const EmptyLabel = styled.div`
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-dim);
  font-style: italic;
`

const blink = keyframes`
  0%, 100% { opacity: 1; }
  50%       { opacity: 0.3; }
`

const PulseDot = styled.div`
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--green);
  animation: ${blink} 2s ease-in-out infinite;
  flex-shrink: 0;
`

const PollLabel = styled.div`
  display: flex;
  align-items: center;
  gap: 5px;
  font-family: var(--font-mono);
  font-size: 9px;
  color: var(--text-dim);
`

function countryFlag(code: string): string {
  if (!code || code.length !== 2) return '🌐'
  return String.fromCodePoint(
    ...code.toUpperCase().split('').map(c => 0x1F1E6 + c.charCodeAt(0) - 65)
  )
}
