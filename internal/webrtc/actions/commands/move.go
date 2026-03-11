package commands

type MoveParams struct {
	TargetX           float64 `json:"x"`
	TargetY           float64 `json:"y"`
	TargetOrientation float64 `json:"r"`
}
