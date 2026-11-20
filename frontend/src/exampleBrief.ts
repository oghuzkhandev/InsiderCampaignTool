// Boş ekranda önerilen örnek brief'ler. Liste `exampleBriefs.json` dosyasındadır; backend
// entegrasyon testi (tests/integration/example-briefs_test.go) aynı dosyayı okuyup her
// örneğin desteklendiğini ve demo verisinde alıcı bulduğunu doğrular. Bu yüzden yeni
// örnek eklerken yalnızca JSON dosyası değiştirilir.
import examples from './exampleBriefs.json'

export interface ExampleBrief {
  id: string
  /** Düğmede görünen kısa ad. */
  title: string
  brief: string
}

export const EXAMPLE_BRIEFS: readonly ExampleBrief[] = examples
