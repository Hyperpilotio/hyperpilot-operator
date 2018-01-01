package common

type StringSet struct {
	set map[string]bool
}

func NewStringSet() *StringSet {
	return &StringSet{make(map[string]bool)}
}

func StringSetFromList(list []string) *StringSet {
	newSet := NewStringSet()
	for _, v := range list {
		newSet.Add(v)
	}
	return newSet
}

func (set *StringSet) Add(s string) bool {
	_, found := set.set[s]
	set.set[s] = true
	return !found
}

func (set *StringSet) IsExist(s string) bool {
	_, found := set.set[s]
	return found
}

func (set *StringSet) Remove(s string) {
	delete(set.set, s)
}

func (set *StringSet) ToList() []string {
	r := []string{}
	for k := range set.set {
		r = append(r, k)
	}
	return r
}

func (set1 *StringSet) IsIdentical(set2 *StringSet) bool {
	if len(set1.set) != len(set2.set) {
		return false
	}
	for k := range set2.set {
		if !set1.IsExist(k) {
			return false
		}
	}
	return true
}

func (set1 *StringSet) Minus(set2 *StringSet) *StringSet {
	result := NewStringSet()
	for k := range set1.set {
		if !set2.IsExist(k) {
			result.Add(k)
		}
	}
	return result
}
