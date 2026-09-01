package weather

// FIT WeatherReport values (fit_profile.json enumeration WeatherReport).
const (
	reportCurrent = 0
	reportHourly  = 1
	reportDaily   = 2
)

// FIT WeatherCondition values (fit_profile.json enumeration WeatherCondition).
const (
	condClear                  = 0
	condPartlyCloudy           = 1
	condMostlyCloudy           = 2
	condRain                   = 3
	condSnow                   = 4
	condWindy                  = 5
	condThunderstorms          = 6
	condWintryMix              = 7
	condFog                    = 8
	condHazy                   = 11
	condHail                   = 12
	condScatteredShowers       = 13
	condScatteredThunderstorms = 14
	condUnknownPrecipitation   = 15
	condLightRain              = 16
	condHeavyRain              = 17
	condLightSnow              = 18
	condHeavySnow              = 19
	condLightRainSnow          = 20
	condHeavyRainSnow          = 21
	condCloudy                 = 22
)

// wmoToOwm maps Open-Meteo WMO weather codes to the OpenWeatherMap codes the
// rest of the pipeline speaks (util/PulseWeather.java:296-315).
func wmoToOwm(wmo int) int {
	switch wmo {
	case 0:
		return 800 // clear
	case 1:
		return 801 // mainly clear
	case 2:
		return 802 // partly cloudy
	case 3:
		return 804 // overcast
	case 45, 48:
		return 741 // fog
	case 51:
		return 300 // light drizzle
	case 53:
		return 301 // drizzle
	case 55:
		return 302 // dense drizzle
	case 56, 57:
		return 511 // freezing drizzle
	case 61:
		return 500 // slight rain
	case 63:
		return 501 // moderate rain
	case 65:
		return 502 // heavy rain
	case 66, 67:
		return 511 // freezing rain
	case 71:
		return 600 // slight snow
	case 73:
		return 601 // moderate snow
	case 75:
		return 602 // heavy snow
	case 77:
		return 611 // snow grains
	case 80:
		return 520 // slight rain showers
	case 81:
		return 521 // moderate rain showers
	case 82:
		return 522 // violent rain showers
	case 85:
		return 620 // slight snow showers
	case 86:
		return 621 // heavy snow showers
	case 95:
		return 211 // thunderstorm
	case 96, 99:
		return 212 // thunderstorm with hail
	}
	return 800
}

// owmToFitCondition maps an OpenWeatherMap code to a FIT WeatherCondition,
// returning nil for the codes Garmin has no bucket for; the encoder then
// writes the invalid marker, same as upstream's 255
// (fieldDefinitions/FieldDefinitionWeatherCondition.java:71-155).
func owmToFitCondition(code int) any {
	switch code {
	case 200, 201, 202, 210, 211, 212, 230, 231, 232, 900, 901, 902, 962:
		return condThunderstorms
	case 221:
		return condScatteredThunderstorms
	case 300, 301, 310, 313, 500, 520, 521:
		return condLightRain
	case 302, 312, 314, 502, 503, 504, 522:
		return condHeavyRain
	case 311, 501, 531:
		return condRain
	case 321:
		return condScatteredShowers
	case 511:
		return condUnknownPrecipitation
	case 600:
		return condLightSnow
	case 601, 620, 621:
		return condSnow
	case 602, 622:
		return condHeavySnow
	case 611, 612, 613:
		return condWintryMix
	case 615:
		return condLightRainSnow
	case 616:
		return condHeavyRainSnow
	case 701, 711, 721, 731, 751, 761, 762:
		return condHazy
	case 741:
		return condFog
	case 771, 781, 905:
		return condWindy
	case 800:
		return condClear
	case 801, 802:
		return condPartlyCloudy
	case 803:
		return condMostlyCloudy
	case 804:
		return condCloudy
	case 906:
		return condHail
	}
	// 903 cold, 904 hot, 951..961 breeze/gale scale: no FIT equivalent.
	return nil
}

// owmToGarminIcon maps an OpenWeatherMap code to the icon id the watch's
// weather widget draws, as observed on a Venu 3
// (http/interceptors/WeatherInterceptor.java:mapToGarminCondition).
func owmToGarminIcon(code int) int {
	switch code {
	case 200, 201, 202, 210, 211, 212, 221, 230, 231, 232:
		return 27 // thunder with rain
	case 771, 781, 900, 901, 902, 905, 951, 952, 953, 954, 955, 956, 957, 958, 959, 960, 961, 962:
		return 46 // wind
	case 300, 301, 302, 310, 311, 312, 313, 314, 321, 500, 501, 502, 503, 504, 520, 521, 522, 531:
		return 17 // rain
	case 511, 615, 616, 906:
		return 40 // snow with rain
	case 600, 601, 602, 611, 612, 620, 621, 622:
		return 38 // snow with clouds
	case 701, 711, 721, 731, 741, 751, 761, 762:
		return 47 // foggy
	case 800, 904:
		return 5 // sunny
	case 801, 802:
		return 8 // sun with clouds
	case 803, 804:
		return 15 // clouds
	}
	return 35 // snowflake, the upstream fallback
}

// wmoText is the condition description shown on the watch
// (util/PulseWeather.java:317-337).
func wmoText(wmo int) string {
	switch wmo {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	}
	return ""
}
