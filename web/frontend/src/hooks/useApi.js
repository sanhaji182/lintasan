import { useState, useEffect, useCallback } from 'react'

export function useApi(url) {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const load = useCallback(() => {
    setLoading(true)
    fetch(url)
      .then(r => r.json())
      .then(d => { setData(d && Object.prototype.hasOwnProperty.call(d, 'data') ? d.data : d); setLoading(false) })
      .catch(e => { setError(e); setLoading(false) })
  }, [url])

  useEffect(() => { load() }, [load])

  return { data, loading, error, reload: load }
}
