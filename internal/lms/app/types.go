package app

// EnvGetterFunc gets variable from environment by its key
type EnvGetterFunc func(key string) string
