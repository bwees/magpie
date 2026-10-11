package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

var ErrInvalidLanguages = errors.New("catalog: invalid wanted languages")

// LanguageOption is one ISO 639-1 language a wanted-language list may hold.
type LanguageOption struct {
	Code domain.Language
	Name string
}

// LanguageOptions are the ISO 639-1 languages, by code.
var LanguageOptions = []LanguageOption{
	{"aa", "Afar"}, {"ab", "Abkhazian"}, {"ae", "Avestan"}, {"af", "Afrikaans"}, {"ak", "Akan"},
	{"am", "Amharic"}, {"an", "Aragonese"}, {"ar", "Arabic"}, {"as", "Assamese"}, {"av", "Avaric"},
	{"ay", "Aymara"}, {"az", "Azerbaijani"}, {"ba", "Bashkir"}, {"be", "Belarusian"}, {"bg", "Bulgarian"},
	{"bi", "Bislama"}, {"bm", "Bambara"}, {"bn", "Bengali"}, {"bo", "Tibetan"}, {"br", "Breton"},
	{"bs", "Bosnian"}, {"ca", "Catalan"}, {"ce", "Chechen"}, {"ch", "Chamorro"}, {"co", "Corsican"},
	{"cr", "Cree"}, {"cs", "Czech"}, {"cu", "Church Slavic"}, {"cv", "Chuvash"}, {"cy", "Welsh"},
	{"da", "Danish"}, {"de", "German"}, {"dv", "Divehi"}, {"dz", "Dzongkha"}, {"ee", "Ewe"},
	{"el", "Greek"}, {"en", "English"}, {"eo", "Esperanto"}, {"es", "Spanish"}, {"et", "Estonian"},
	{"eu", "Basque"}, {"fa", "Persian"}, {"ff", "Fulah"}, {"fi", "Finnish"}, {"fj", "Fijian"},
	{"fo", "Faroese"}, {"fr", "French"}, {"fy", "Western Frisian"}, {"ga", "Irish"}, {"gd", "Scottish Gaelic"},
	{"gl", "Galician"}, {"gn", "Guarani"}, {"gu", "Gujarati"}, {"gv", "Manx"}, {"ha", "Hausa"},
	{"he", "Hebrew"}, {"hi", "Hindi"}, {"ho", "Hiri Motu"}, {"hr", "Croatian"}, {"ht", "Haitian"},
	{"hu", "Hungarian"}, {"hy", "Armenian"}, {"hz", "Herero"}, {"ia", "Interlingua"}, {"id", "Indonesian"},
	{"ie", "Interlingue"}, {"ig", "Igbo"}, {"ii", "Sichuan Yi"}, {"ik", "Inupiaq"}, {"io", "Ido"},
	{"is", "Icelandic"}, {"it", "Italian"}, {"iu", "Inuktitut"}, {"ja", "Japanese"}, {"jv", "Javanese"},
	{"ka", "Georgian"}, {"kg", "Kongo"}, {"ki", "Kikuyu"}, {"kj", "Kuanyama"}, {"kk", "Kazakh"},
	{"kl", "Kalaallisut"}, {"km", "Khmer"}, {"kn", "Kannada"}, {"ko", "Korean"}, {"kr", "Kanuri"},
	{"ks", "Kashmiri"}, {"ku", "Kurdish"}, {"kv", "Komi"}, {"kw", "Cornish"}, {"ky", "Kyrgyz"},
	{"la", "Latin"}, {"lb", "Luxembourgish"}, {"lg", "Ganda"}, {"li", "Limburgish"}, {"ln", "Lingala"},
	{"lo", "Lao"}, {"lt", "Lithuanian"}, {"lu", "Luba-Katanga"}, {"lv", "Latvian"}, {"mg", "Malagasy"},
	{"mh", "Marshallese"}, {"mi", "Maori"}, {"mk", "Macedonian"}, {"ml", "Malayalam"}, {"mn", "Mongolian"},
	{"mr", "Marathi"}, {"ms", "Malay"}, {"mt", "Maltese"}, {"my", "Burmese"}, {"na", "Nauru"},
	{"nb", "Norwegian Bokmål"}, {"nd", "North Ndebele"}, {"ne", "Nepali"}, {"ng", "Ndonga"}, {"nl", "Dutch"},
	{"nn", "Norwegian Nynorsk"}, {"no", "Norwegian"}, {"nr", "South Ndebele"}, {"nv", "Navajo"}, {"ny", "Chichewa"},
	{"oc", "Occitan"}, {"oj", "Ojibwa"}, {"om", "Oromo"}, {"or", "Oriya"}, {"os", "Ossetian"},
	{"pa", "Punjabi"}, {"pi", "Pali"}, {"pl", "Polish"}, {"ps", "Pashto"}, {"pt", "Portuguese"},
	{"qu", "Quechua"}, {"rm", "Romansh"}, {"rn", "Rundi"}, {"ro", "Romanian"}, {"ru", "Russian"},
	{"rw", "Kinyarwanda"}, {"sa", "Sanskrit"}, {"sc", "Sardinian"}, {"sd", "Sindhi"}, {"se", "Northern Sami"},
	{"sg", "Sango"}, {"si", "Sinhala"}, {"sk", "Slovak"}, {"sl", "Slovenian"}, {"sm", "Samoan"},
	{"sn", "Shona"}, {"so", "Somali"}, {"sq", "Albanian"}, {"sr", "Serbian"}, {"ss", "Swati"},
	{"st", "Southern Sotho"}, {"su", "Sundanese"}, {"sv", "Swedish"}, {"sw", "Swahili"}, {"ta", "Tamil"},
	{"te", "Telugu"}, {"tg", "Tajik"}, {"th", "Thai"}, {"ti", "Tigrinya"}, {"tk", "Turkmen"},
	{"tl", "Tagalog"}, {"tn", "Tswana"}, {"to", "Tonga"}, {"tr", "Turkish"}, {"ts", "Tsonga"},
	{"tt", "Tatar"}, {"tw", "Twi"}, {"ty", "Tahitian"}, {"ug", "Uyghur"}, {"uk", "Ukrainian"},
	{"ur", "Urdu"}, {"uz", "Uzbek"}, {"ve", "Venda"}, {"vi", "Vietnamese"}, {"vo", "Volapük"},
	{"wa", "Walloon"}, {"wo", "Wolof"}, {"xh", "Xhosa"}, {"yi", "Yiddish"}, {"yo", "Yoruba"},
	{"za", "Zhuang"}, {"zh", "Chinese"}, {"zu", "Zulu"},
}

func knownLanguage(code domain.Language) bool {
	return slices.ContainsFunc(LanguageOptions, func(o LanguageOption) bool { return o.Code == code })
}

// validateLanguages accepts a non-empty list of distinct ISO 639-1 languages.
func validateLanguages(langs []api.WantedLanguage) error {
	if len(langs) == 0 {
		return fmt.Errorf("%w: at least one language is required", ErrInvalidLanguages)
	}
	seen := map[domain.Language]bool{}
	for _, lang := range langs {
		if !knownLanguage(lang.Language) {
			return fmt.Errorf("%w: %q is not an ISO 639-1 language code", ErrInvalidLanguages, lang.Language)
		}
		if seen[lang.Language] {
			return fmt.Errorf("%w: %q is listed twice", ErrInvalidLanguages, lang.Language)
		}
		seen[lang.Language] = true
	}
	return nil
}

// SetGlobalLanguages replaces the global wanted-language list and publishes LanguagesChanged.
func (s *Service) SetGlobalLanguages(ctx context.Context, langs []api.WantedLanguage) ([]api.WantedLanguage, error) {
	if err := validateLanguages(langs); err != nil {
		return nil, err
	}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.repo.replaceGlobalLanguages(tx, langs); err != nil {
			return err
		}
		return s.publish(tx, api.LanguagesChanged{Languages: langs})
	})
	return langs, err
}

// SetTitleLanguages sets a Title's override, or clears it for nil, and publishes LanguagesChanged with the
// Title's new effective list.
func (s *Service) SetTitleLanguages(ctx context.Context, id domain.TitleID, langs []api.WantedLanguage) (TitleView, error) {
	if langs != nil {
		if err := validateLanguages(langs); err != nil {
			return TitleView{}, err
		}
	}
	var view TitleView
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		effective := langs
		row.LanguagesOverride = nil
		if langs == nil {
			if effective, err = s.repo.globalLanguages(tx); err != nil {
				return err
			}
		} else {
			encoded, err := json.Marshal(langs)
			if err != nil {
				return err
			}
			row.LanguagesOverride = ptr(string(encoded))
		}
		if err := s.repo.saveTitle(tx, row); err != nil {
			return err
		}
		if err := s.publish(tx, api.LanguagesChanged{Title: &id, Languages: effective}); err != nil {
			return err
		}
		view, err = s.view(tx, id)
		return err
	})
	return view, err
}
