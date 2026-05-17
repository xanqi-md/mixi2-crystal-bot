require "http/client"
require "json"
require "file_utils"
require "set"

class CardCandidate
  getter name : String
  getter image_url : String
  getter source_url : String?
  getter card_type : String
  getter property_type : String?
  getter attribute : String?
  getter races : Array(String)
  getter specs : Array(String)
  getter level : Int32?
  getter rank : Int32?
  getter attack : Int32?
  getter defense : Int32?
  getter rarity : String?
  getter card_text : String?

  def initialize(
    @name : String,
    @image_url : String,
    @source_url : String?,
    @card_type : String,
    @property_type : String?,
    @attribute : String?,
    @races : Array(String),
    @specs : Array(String),
    @level : Int32?,
    @rank : Int32?,
    @attack : Int32?,
    @defense : Int32?,
    @rarity : String?,
    @card_text : String?
  )
  end
end

class DailyCardPayload
  include JSON::Serializable

  getter text : String
  getter card_name : String
  getter image_url : String
  getter source_url : String?
  getter date_key : String
  getter media_description : String

  def initialize(
    @text : String,
    @card_name : String,
    @image_url : String,
    @source_url : String?,
    @date_key : String,
    @media_description : String
  )
  end
end

class DailyCardSelector
  DEFAULT_HTML_URL       = "https://ygogenesys-ja.com/"
  DEFAULT_IMAGE_BASE_URL = "https://images.ygoprodeck.com/images/cards_cropped"

  def initialize
    @html_url = ENV["DAILY_CARD_HTML_URL"]?
    @html_file = ENV["DAILY_CARD_HTML_FILE"]?
    @image_base_url = ENV["DAILY_CARD_IMAGE_BASE_URL"]? || DEFAULT_IMAGE_BASE_URL
    @output_path = ENV["DAILY_CARD_PAYLOAD_PATH"]? || "/tmp/daily_card_payload.json"
    @post_label = ENV["DAILY_CARD_LABEL"]? || "今日の1枚\n画像の左下、【ALT】ボタンよりカード詳細を確認できます！\n"
  end

  def run
    html, source_url = load_html_and_source
    cards = extract_cards_from_const_cards(html, source_url)

    if cards.empty?
      STDERR.puts "カード候補を抽出できませんでした。const CARDS の構造を確認してください。"
      exit 1
    end

    date_key = jst_date_key
    chosen = select_card(cards, date_key)
    media_description = build_media_description(chosen)

    payload = DailyCardPayload.new(
      "#{@post_label}\n#{chosen.name}",
      chosen.name,
      chosen.image_url,
      chosen.source_url,
      date_key,
      media_description
    )

    FileUtils.mkdir_p(File.dirname(@output_path))
    File.write(@output_path, payload.to_json)

    puts "候補数: #{cards.size}"
    puts "選択日: #{date_key}"
    puts "カード名: #{chosen.name}"
    puts "画像URL: #{chosen.image_url}"
    puts "payload: #{@output_path}"
  end

  private def load_html_and_source : Tuple(String, String?)
    if html_file = @html_file
      return {File.read(html_file), nil}
    end

    target_url = @html_url || DEFAULT_HTML_URL
    response = HTTP::Client.get(
      target_url,
      headers: HTTP::Headers{
        "User-Agent" => "mixi2-crystal-bot/1.0",
      }
    )

    unless response.success?
      raise "HTML 取得失敗: #{response.status_code} #{response.status_message}"
    end

  base_url = target_url.ends_with?("/") ? target_url[0...-1] : target_url
  script_url = "#{base_url}/script.js"
  
  script_response = HTTP::Client.get(
    script_url,
    headers: HTTP::Headers{
      "User-Agent" => "mixi2-crystal-bot/1.0",
    }
  )

  html_content = response.body
  if script_response.success?
    html_content = "#{html_content}\n#{script_response.body}"
  end

  {html_content, target_url}
end

private def extract_cards_from_const_cards(html : String, source_url : String?) : Array(CardCandidate)
  json_array_text = extract_cards_json_array(html)
  raw_cards = JSON.parse(json_array_text).as_a

  cards = [] of CardCandidate
  seen = Set(String).new

  raw_cards.each do |entry|
    obj = entry.as_h

    name = clean_text(string_field(obj, "name") || "")
    image_id = (string_field(obj, "file_name") || "").strip
    card_type = clean_text(string_field(obj, "card_type") || "")

    next if name.empty? || image_id.empty? || card_type.empty?

    # monster_type_lineから種族と分類を抽出
    monster_type_line = clean_text(string_field(obj, "monster_type_line") || "")
    fallback_parts = monster_type_line.split("/").map(&.strip).reject(&.empty?)
    
    races = fallback_parts.size > 0 ? [fallback_parts[0]] : [] of String
    specs = fallback_parts.size > 1 ? fallback_parts[1, fallback_parts.size - 1] : [] of String

    property_type = blank_to_nil(clean_text(string_field(obj, "property") || ""))
    attribute = blank_to_nil(clean_text(string_field(obj, "attribute") || ""))
    rarity = blank_to_nil(clean_text(string_field(obj, "master_duel_rarity") || ""))
    card_text = blank_to_nil(clean_text(string_field(obj, "text") || ""))

    image_url = build_image_url(image_id)
    key = "#{name}\u0000#{image_url}"
    next if seen.includes?(key)

    seen.add(key)
    cards << CardCandidate.new(
      name,
      image_url,
      source_url,
      card_type,
      property_type,
      attribute,
      races,
      specs,
      int_field(obj, "level"),
      int_field(obj, "rank"),
      int_field(obj, "atk"),
      int_field(obj, "def"),
      rarity,
      card_text
    )
  end

  cards
end

  private def extract_cards_json_array(html : String) : String
    start_marker = "const RAW_CARDS = "
    
    start_index = html.index(start_marker)
    raise "#{start_marker} が見つかりませんでした" unless start_index

    body_start = start_index + start_marker.size

    # JSONの配列を正確に抽出するため、括弧のバランスを取る
    bracket_count = 0
    in_string = false
    escape_next = false
    end_index : Int32? = nil

    html.each_char_with_index do |char, i|
      next if i < body_start

      if escape_next
        escape_next = false
        next
      end

      if char == '\\'
        escape_next = true
        next
      end

      if char == '"' && !escape_next
        in_string = !in_string
        next
      end

      next if in_string

      if char == '['
        bracket_count += 1
      elsif char == ']'
        bracket_count -= 1
        if bracket_count == 0
          end_index = i + 1
          break
        end
      end
    end

    raise "RAW_CARDS の終端が見つかりませんでした" unless end_index

    json_text = html[body_start...end_index.not_nil!].strip

    unless json_text.starts_with?("[") && json_text.ends_with?("]")
      raise "RAW_CARDS JSON の切り出しに失敗しました。先頭: #{json_text[0..50]?}"
    end

    json_text
  end


  private def build_media_description(card : CardCandidate) : String
    lines = [] of String

    lines << "カード名: #{card.name}"
    lines << "カード種別: #{card.card_type}"

    if card.card_type == "モンスター"
      lines << "属性: #{card.attribute.not_nil!}" if present?(card.attribute)
      lines << "種族: #{card.races.join(" / ")}" unless card.races.empty?
      lines << "分類: #{card.specs.join(" / ")}" unless card.specs.empty?

      if card.rank
        lines << "ランク: #{card.rank}"
      elsif card.level
        lines << "レベル: #{card.level}"
      end

      if !card.attack.nil? || !card.defense.nil?
        lines << "ATK/DEF: #{display_stat(card.attack)} / #{display_stat(card.defense)}"
      end
    else
      lines << "種類: #{card.property_type.not_nil!}" if present?(card.property_type)
    end

    lines << "レアリティ: #{card.rarity.not_nil!}" if present?(card.rarity)
    lines << "カードテキスト: #{card.card_text.not_nil!}" if present?(card.card_text)

    lines.join("\n")
  end

  private def build_image_url(image_id : String) : String
    "#{@image_base_url}/#{image_id}.jpg"
  end

  private def clean_text(text : String) : String
    text
      .gsub(/<[^>]+>/, "")
      .gsub(/\s+/, " ")
      .strip
  end

  private def string_field(obj : Hash(String, JSON::Any), key : String) : String?
    value = obj[key]?
    return nil unless value
    value.raw.as?(String)
  end

  private def int_field(obj : Hash(String, JSON::Any), key : String) : Int32?
    value = obj[key]?
    return nil unless value

    raw = value.raw
    case raw
    when Int64
      raw.to_i32
    when Int32
      raw
    when Float64
      raw.to_i32
    else
      nil
    end
  end

  private def string_array_field(obj : Hash(String, JSON::Any), key : String) : Array(String)
    value = obj[key]?
    return [] of String unless value

    arr = value.as_a?
    return [] of String unless arr

    arr.compact_map do |item|
      str = item.raw.as?(String)
      str ? clean_text(str) : nil
    end
  end

  private def blank_to_nil(value : String) : String?
    stripped = value.strip
    stripped.empty? ? nil : stripped
  end

  private def present?(value : String?) : Bool
    !value.nil? && !value.not_nil!.strip.empty?
  end

  private def display_stat(value : Int32?) : String
    value ? value.to_s : "-"
  end

  private def jst_date_key : String
    (Time.utc + 9.hours).to_s("%Y-%m-%d")
  end

  private def select_card(cards : Array(CardCandidate), date_key : String) : CardCandidate
    index = stable_index(date_key, cards.size)
    cards[index]
  end

  private def stable_index(text : String, size : Int32) : Int32
    hash = 2166136261_u32

    text.each_byte do |byte|
      hash ^= byte.to_u32
      hash &*= 16777619_u32
    end

    (hash % size.to_u32).to_i32
  end
end

DailyCardSelector.new.run
