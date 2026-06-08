export interface EventProps {
  id: number
  city: string
  country: string
  country_code: string
  path: string
  method: string
  status_code: number
  created_at: string
  ip: string
}

export interface GeoJSONFeature {
  type: 'Feature'
  geometry: {
    type: 'Point'
    coordinates: [number, number] // [lon, lat]
  }
  properties: EventProps
}

export interface FeatureCollection {
  type: 'FeatureCollection'
  features: GeoJSONFeature[]
}

export interface StatsResponse {
  countries: Record<string, number>
  req_per_min: number
  total_in_window: number
}
