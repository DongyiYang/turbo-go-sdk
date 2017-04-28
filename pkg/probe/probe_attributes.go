package probe


type ProbeSignature struct {
	ProbeType     string
	ProbeCategory string
}

type ProbeProperties struct {
	ProbeSignature *ProbeSignature
	Probe          *TurboProbe
}
