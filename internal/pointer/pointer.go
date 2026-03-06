package pointer

func To[A any](a A) *A {
	return &a
}

func ValStr[T any](p *T) any {
	if p == nil {
		return "<nil>"
	}
	return *p
}
