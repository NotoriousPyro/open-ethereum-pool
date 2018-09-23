package util

import (
    "math/big"
    "regexp"
    "strconv"
    "time"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/common/math"
)

var (
    Pow256 = math.BigPow(2, 256)
    Satoshi = math.BigPow(10, 8)
    addressPattern = regexp.MustCompile("^M[A-Z0-9]{1}[0-9a-zA-Z]{32}$")
    zeroHash = regexp.MustCompile("^0?x?0+$")
)

func IsValidHexAddress(s string) bool {
    if !addressPattern.MatchString(s) {
        return false
    }
    return true
}

func IsZeroHash(s string) bool {
    return zeroHash.MatchString(s)
}

func MakeTimestamp() int64 {
    return time.Now().UnixNano() / int64(time.Millisecond)
}

func GetTargetHex(diff int64) string {
    difficulty := big.NewInt(diff)
    diff1 := new(big.Int).Div(Pow256, difficulty)
    return string(common.ToHex(diff1.Bytes()))
}

func TargetHexToDiff(targetHex string) *big.Int {
    targetBytes := common.FromHex(targetHex)
    return new(big.Int).Div(Pow256, new(big.Int).SetBytes(targetBytes))
}

func ToHex(n int64) string {
    return "0x0" + strconv.FormatInt(n, 16)
}

func FormatReward(reward *big.Int) string {
    return reward.String()
}

func FormatRatReward(reward *big.Rat) string {
    return reward.FloatString(8)
}

func StringInSlice(a string, list []string) bool {
    for _, b := range list {
        if b == a {
            return true
        }
    }
    return false
}

func MustParseDuration(s string) time.Duration {
    value, err := time.ParseDuration(s)
    if err != nil {
        panic("util: Can't parse duration `" + s + "`: " + err.Error())
    }
    return value
}
